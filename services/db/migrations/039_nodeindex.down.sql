drop table `node_index`;

drop index cvm_transactions_txdata_from_created_at
    on cvm_transactions_txdata;

drop index cvm_transactions_txdata_to_created_at
    on cvm_transactions_txdata;

create index cvm_transactions_txdata_from
    on cvm_transactions_txdata (from_addr);

create index cvm_transactions_txdata_to
    on cvm_transactions_txdata (to_addr);

alter table `rewards` drop COLUMN `processed`;
drop index rewards_processed on rewards;
