create table `node_index`
(
    instance varchar(100) not null,
    topic varchar(100) not null,
    idx   bigint unsigned default 0,
    primary key (instance, topic)
);

drop index cvm_transactions_txdata_from
    on cvm_transactions_txdata;

drop index cvm_transactions_txdata_to
    on cvm_transactions_txdata;

create index cvm_transactions_txdata_from_created_at
    on cvm_transactions_txdata (from_addr, created_at);

create index cvm_transactions_txdata_to_created_at
    on cvm_transactions_txdata (to_addr, created_at);

alter table `rewards` add COLUMN `processed` smallint unsigned default 0;
create index rewards_processed on rewards (processed, created_at);



