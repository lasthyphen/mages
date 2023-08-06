DROP INDEX cvm_transactions_txdata_from ON cvm_transactions_txdata;
DROP INDEX cvm_transactions_txdata_to ON cvm_transactions_txdata;
DROP INDEX cvm_transactions_txdata_created_at ON cvm_transactions_txdata;

CREATE INDEX cvm_transactions_txdata_from_created_at ON cvm_transactions_txdata (from_addr, created_at);
CREATE INDEX cvm_transactions_txdata_to_created_at ON cvm_transactions_txdata (to_addr, created_at);
