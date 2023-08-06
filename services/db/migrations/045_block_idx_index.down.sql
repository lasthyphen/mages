DROP INDEX cvm_transactions_txdata_block_idx on cvm_transactions_txdata;
ALTER TABLE cvm_transactions_txdata DROP COLUMN block_idx;
CREATE UNIQUE INDEX cvm_transactions_txdata_block ON cvm_transactions_txdata (block, idx);