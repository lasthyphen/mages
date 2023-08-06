ALTER TABLE cvm_transactions_txdata ADD COLUMN `amount` bigInt unsigned not null default 0 AFTER `nonce`;
ALTER TABLE cvm_transactions_txdata ADD COLUMN `status` smallInt unsigned not null default 0 AFTER `amount`;
ALTER TABLE cvm_transactions_txdata ADD COLUMN `gas_price` bigInt unsigned not null default 0 AFTER `status`;
ALTER TABLE cvm_transactions_txdata ADD COLUMN `gas_used` int unsigned not null default 0 AFTER `gas_price`;
ALTER TABLE cvm_transactions_txdata ADD COLUMN `receipt` mediumblob AFTER `serialization`;

DROP TABLE cvm_transactions_receipts;
CREATE INDEX cvm_blocks_chain_id ON cvm_blocks (chain_id);