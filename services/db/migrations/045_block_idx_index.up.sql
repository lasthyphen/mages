ALTER TABLE cvm_transactions_txdata MODIFY block BigInt UNSIGNED NOT NULL;
ALTER TABLE cvm_transactions_txdata MODIFY idx SmallInt UNSIGNED NOT NULL;
DROP INDEX cvm_transactions_txdata_block on cvm_transactions_txdata;
ALTER TABLE cvm_transactions_txdata ADD COLUMN block_idx BIGINT UNSIGNED GENERATED ALWAYS AS (BLOCK*1000 + cast(999 - cast(idx AS SIGNED) AS UNSIGNED)) STORED;
CREATE UNIQUE INDEX cvm_transactions_txdata_block_idx ON cvm_transactions_txdata (block_idx);