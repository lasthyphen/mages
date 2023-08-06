// (c) 2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/lasthyphen/dijetsnodego/utils/logging"
	"github.com/lasthyphen/mages/api"
	"github.com/lasthyphen/mages/balance"
	"github.com/lasthyphen/mages/caching"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services/rewards"
	"github.com/lasthyphen/mages/servicesctrl"
	"github.com/lasthyphen/mages/stream"
	"github.com/lasthyphen/mages/stream/consumers"
	"github.com/lasthyphen/mages/utils"
	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	magellanRpc "github.com/lasthyphen/mages/rpc"
	_ "github.com/golang-migrate/migrate/v4/database"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const (
	rootCmdUse  = "magelland [command]"
	rootCmdDesc = "Daemons for Magellan."

	apiCmdUse  = "api"
	apiCmdDesc = "Runs the API daemon"

	streamCmdUse  = "stream"
	streamCmdDesc = "Runs stream commands"

	streamIndexerCmdUse  = "indexer"
	streamIndexerCmdDesc = "Runs the stream indexer daemon"

	envCmdUse  = "env"
	envCmdDesc = "Displays information about the Magellan environment"

	defaultReplayQueueSize    = int(2000)
	defaultReplayQueueThreads = int(4)

	mysqlMigrationFlag          = "run-mysql-migration"
	mysqlMigrationFlagShorthand = "m"
	mysqlMigrationFlagDesc      = "Executes migration scripts for the configured mysql database. This might change the schema, use with caution. For more info on migration have a look at the migrations folder in services/db/migrations/ and check the latest commits"

	mysqlMigrationPathFlag          = "mysql-migration-path"
	mysqlMigrationPathFlagShorthand = "p"
	mysqlMigrationPathDefault       = "services/db/migrations"
)

func main() {
	if err := execute(); err != nil {
		log.Fatalln("Failed to run:", err.Error())
	}
}

// Get the root command for magellan
func execute() error {
	rand.Seed(time.Now().UnixNano())

	var (
		runErr             error
		config             = &cfg.Config{}
		serviceControl     = &servicesctrl.Control{}
		runMysqlMigration  = func() *bool { s := false; return &s }()
		mysqlMigrationPath = func() *string { s := mysqlMigrationPathDefault; return &s }()
		configFile         = func() *string { s := ""; return &s }()
		replayqueuesize    = func() *int { i := defaultReplayQueueSize; return &i }()
		replayqueuethreads = func() *int { i := defaultReplayQueueThreads; return &i }()
		cmd                = &cobra.Command{
			Use: rootCmdUse, Short: rootCmdDesc, Long: rootCmdDesc,
			PersistentPreRun: func(cmd *cobra.Command, args []string) {
				c, err := cfg.NewFromFile(*configFile)
				if err != nil {
					log.Fatalln("Failed to read config file", *configFile, ":", err.Error())
				}
				lf := logging.NewFactory(c.Logging)
				alog, err := lf.Make("magellan")
				if err != nil {
					log.Fatalln("Failed to create log", c.Logging.Directory, ":", err.Error())
				}

				mysqllogger := &MysqlLogger{
					Log: alog,
				}
				_ = mysql.SetLogger(mysqllogger)

				if *runMysqlMigration {
					dbConfig := c.Services.DB
					migrationErr := migrateMysql(fmt.Sprintf("%s://%s", dbConfig.Driver, dbConfig.DSN), *mysqlMigrationPath)
					if migrationErr != nil {
						log.Fatalf("Failed to run migration: %v", migrationErr)
					}
				}

				models.SetBech32HRP(c.NetworkID)

				serviceControl.Log = alog
				serviceControl.Services = c.Services
				serviceControl.ServicesCfg = *c
				serviceControl.Chains = c.Chains
				serviceControl.Persist = db.NewPersist()
				serviceControl.Features = c.Features
				persist := db.NewPersist()
				serviceControl.BalanceManager = balance.NewManager(persist, serviceControl)
				serviceControl.AggregatesCache = caching.NewAggregatesCache()
				err = serviceControl.Init(c.NetworkID)
				if err != nil {
					log.Fatalln("Failed to create service control", ":", err.Error())
				}

				*config = *c

				if config.MetricsListenAddr != "" {
					sm := http.NewServeMux()
					sm.Handle("/metrics", promhttp.Handler())
					go func() {
						server := &http.Server{
							Addr:              config.MetricsListenAddr,
							Handler:           sm,
							ReadHeaderTimeout: 5 * time.Second,
						}
						err = server.ListenAndServe()
						if err != nil {
							log.Fatalln("Failed to start metrics listener", err.Error())
						}
					}()
					alog.Info("starting metrics handler",
						zap.String("addr", config.MetricsListenAddr),
					)
				}
				if config.AdminListenAddr != "" {
					rpcServer := rpc.NewServer()
					codec := json2.NewCodec()
					rpcServer.RegisterCodec(codec, "application/json")
					rpcServer.RegisterCodec(codec, "application/json;charset=UTF-8")
					api := magellanRpc.NewAPI(alog)
					if err := rpcServer.RegisterService(api, "api"); err != nil {
						log.Fatalln("Failed to start admin listener", err.Error())
					}
					sm := http.NewServeMux()
					sm.Handle("/api", rpcServer)
					go func() {
						server := &http.Server{
							Handler:           sm,
							Addr:              config.AdminListenAddr,
							ReadHeaderTimeout: 5 * time.Second,
						}
						err = server.ListenAndServe()
						if err != nil {
							log.Fatalln("Failed to start metrics listener", err.Error())
						}
					}()
				}
			},
		}
	)

	// Add flags and commands
	cmd.PersistentFlags().StringVarP(configFile, "config", "c", "config.json", "config file")
	cmd.PersistentFlags().IntVarP(replayqueuesize, "replayqueuesize", "", defaultReplayQueueSize, fmt.Sprintf("replay queue size default %d", defaultReplayQueueSize))
	cmd.PersistentFlags().IntVarP(replayqueuethreads, "replayqueuethreads", "", defaultReplayQueueThreads, fmt.Sprintf("replay queue size threads default %d", defaultReplayQueueThreads))

	// migration specific flags
	cmd.PersistentFlags().BoolVarP(runMysqlMigration, mysqlMigrationFlag, mysqlMigrationFlagShorthand, false, mysqlMigrationFlagDesc)
	cmd.PersistentFlags().StringVarP(mysqlMigrationPath, mysqlMigrationPathFlag, mysqlMigrationPathFlagShorthand, mysqlMigrationPathDefault, "path for mysql migrations")

	cmd.AddCommand(
		createStreamCmds(serviceControl, config, &runErr),
		createAPICmds(serviceControl, config, &runErr),
		createEnvCmds(config, &runErr))

	// Execute the command and return the runErr to the caller
	if err := cmd.Execute(); err != nil {
		return err
	}

	return runErr
}

func createAPICmds(sc *servicesctrl.Control, config *cfg.Config, runErr *error) *cobra.Command {
	return &cobra.Command{
		Use:   apiCmdUse,
		Short: apiCmdDesc,
		Long:  apiCmdDesc,
		Run: func(cmd *cobra.Command, args []string) {
			go func() {
				err := sc.StartCacheScheduler(config)
				if err != nil {
					return
				}
			}()
			lc, err := api.NewServer(sc, *config)
			if err != nil {
				*runErr = err
				return
			}
			runListenCloser(lc)
		},
	}
}

func createStreamCmds(sc *servicesctrl.Control, config *cfg.Config, runErr *error) *cobra.Command {
	streamCmd := &cobra.Command{
		Use:   streamCmdUse,
		Short: streamCmdDesc,
		Long:  streamCmdDesc,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
			os.Exit(0)
		},
	}

	// Add sub commands
	streamCmd.AddCommand(&cobra.Command{
		Use:   streamIndexerCmdUse,
		Short: streamIndexerCmdDesc,
		Long:  streamIndexerCmdDesc,
		Run: func(cmd *cobra.Command, arg []string) {
			runStreamProcessorManagers(
				sc,
				config,
				runErr,
				producerFactories(sc, config),
				[]consumers.ConsumerFactory{
					consumers.IndexerConsumer,
				},
				[]stream.ProcessorFactoryChainDB{
					consumers.IndexerDB,
					consumers.IndexerConsensusDB,
				},
				[]stream.ProcessorFactoryInstDB{},
			)(cmd, arg)
		},
	})

	return streamCmd
}

func producerFactories(sc *servicesctrl.Control, cfg *cfg.Config) []utils.ListenCloser {
	var factories []utils.ListenCloser
	for _, v := range cfg.Chains {
		switch v.VMType {
		case models.AVMName:
			p, err := stream.NewProducerChain(sc, *cfg, v.ID, stream.EventTypeDecisions, stream.IndexTypeTransactions, stream.IndexXChain)
			if err != nil {
				panic(err)
			}
			factories = append(factories, p)
			p, err = stream.NewProducerChain(sc, *cfg, v.ID, stream.EventTypeConsensus, stream.IndexTypeVertices, stream.IndexXChain)
			if err != nil {
				panic(err)
			}
			factories = append(factories, p)
		case models.PVMName:
			p, err := stream.NewProducerChain(sc, *cfg, v.ID, stream.EventTypeDecisions, stream.IndexTypeBlocks, stream.IndexPChain)
			if err != nil {
				panic(err)
			}
			factories = append(factories, p)
		case models.CVMName:
			p, err := stream.NewProducerChain(sc, *cfg, v.ID, stream.EventTypeDecisions, stream.IndexTypeBlocks, stream.IndexCChain)
			if err != nil {
				panic(err)
			}
			factories = append(factories, p)
		}
	}
	return factories
}

func createEnvCmds(config *cfg.Config, runErr *error) *cobra.Command {
	return &cobra.Command{
		Use:   envCmdUse,
		Short: envCmdDesc,
		Long:  envCmdDesc,
		Run: func(_ *cobra.Command, _ []string) {
			configBytes, err := json.MarshalIndent(config, "", "    ")
			if err != nil {
				*runErr = err
				return
			}

			fmt.Println(string(configBytes))
		},
	}
}

// runListenCloser runs the ListenCloser until signaled to stop
func runListenCloser(lc utils.ListenCloser) {
	// Start listening in the background
	go func() {
		if err := lc.Listen(); err != nil {
			log.Fatalln("Daemon listen error:", err.Error())
		}
	}()

	// Wait for exit signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sigCh

	// Stop server
	if err := lc.Close(); err != nil {
		log.Fatalln("Daemon shutdown error:", err.Error())
	}
}

// runStreamProcessorManagers returns a cobra command that instantiates and runs
// a set of stream process managers
func runStreamProcessorManagers(
	sc *servicesctrl.Control,
	config *cfg.Config,
	runError *error,
	listenCloseFactories []utils.ListenCloser,
	consumerFactories []consumers.ConsumerFactory,
	factoriesChainDB []stream.ProcessorFactoryChainDB,
	factoriesInstDB []stream.ProcessorFactoryInstDB,
) func(_ *cobra.Command, _ []string) {
	return func(_ *cobra.Command, arg []string) {
		wg := &sync.WaitGroup{}

		bm, _ := sc.BalanceManager.(*balance.Manager)
		err := bm.Start(sc.IsAccumulateBalanceIndexer)
		if err != nil {
			*runError = err
			return
		}
		defer func() {
			bm.Close()
		}()

		rh := &rewards.Handler{}
		err = rh.Start(sc)
		if err != nil {
			*runError = err
			return
		}
		defer func() {
			rh.Close()
		}()

		err = consumers.Bootstrap(sc, config.NetworkID, config, consumerFactories)
		if err != nil {
			*runError = err
			return
		}

		sc.BalanceManager.Exec()

		runningControl := utils.NewRunning()

		err = consumers.IndexerFactories(sc, config, factoriesChainDB, factoriesInstDB, wg, runningControl)
		if err != nil {
			*runError = err
			return
		}

		if len(arg) > 0 && arg[0] == "api" {
			lc, err := api.NewServer(sc, *config)
			if err != nil {
				log.Fatalln("API listen error:", err.Error())
			} else {
				listenCloseFactories = append(listenCloseFactories, lc)
			}
		}

		for _, listenCloseFactory := range listenCloseFactories {
			wg.Add(1)
			go func(lc utils.ListenCloser) {
				wg.Done()
				if err := lc.Listen(); err != nil {
					log.Fatalln("Daemon listen error:", err.Error())
				}
			}(listenCloseFactory)
		}

		// Wait for exit signal
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		<-sigCh

		for _, listenCloseFactory := range listenCloseFactories {
			if err := listenCloseFactory.Close(); err != nil {
				log.Println("Daemon shutdown error:", err.Error())
			}
		}

		runningControl.Close()

		wg.Wait()
	}
}

type MysqlLogger struct {
	Log logging.Logger
}

func (m *MysqlLogger) Print(v ...interface{}) {
	s := fmt.Sprint(v...)
	m.Log.Warn("mysql",
		zap.String("body", s),
	)
}

func migrateMysql(mysqlDSN, migrationsPath string) error {
	migrationSource := fmt.Sprintf("file://%v", migrationsPath)
	migrater, migraterErr := migrate.New(migrationSource, mysqlDSN)
	if migraterErr != nil {
		return migraterErr
	}

	if err := migrater.Up(); err != nil {
		if err != migrate.ErrNoChange {
			return err
		}
		log.Println("[mysql]: no new migrations to execute")
	}

	return nil
}
