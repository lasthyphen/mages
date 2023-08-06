CREATE TABLE cvm_accounts (address CHAR(42), tx_count BIGINT UNSIGNED NOT NULL DEFAULT 0, creation_tx char(66));
CREATE UNIQUE INDEX cvm_accounts_address ON cvm_accounts (address);

INSERT INTO cvm_accounts (address, creation_tx) SELECT JSON_VALUE(CONVERT(receipt USING utf8mb4), '$.contractAddress'), hash from cvm_transactions_txdata WHERE NULLIF(to_addr,"") IS NULL;
INSERT INTO cvm_accounts (address,tx_count) SELECT * FROM(SELECT from_addr, COUNT(from_addr) as count from cvm_transactions_txdata GROUP by from_addr) AS N ON DUPLICATE KEY UPDATE tx_count=tx_count+N.count;
INSERT INTO cvm_accounts (address,tx_count) SELECT * FROM(SELECT NULLIF(to_addr,"") AS T, COUNT(COALESCE(to_addr,"")) as count from cvm_transactions_txdata WHERE from_addr != COALESCE(to_addr,"") GROUP by T) AS N ON DUPLICATE KEY UPDATE tx_count=tx_count+N.count;
ALTER TABLE cvm_accounts ADD id BigInt UNSIGNED NOT NULL PRIMARY KEY AUTO_INCREMENT FIRST;

ALTER TABLE cvm_transactions_txdata ADD COLUMN id_from_addr BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER from_addr, ADD COLUMN id_to_addr BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER to_addr;
UPDATE cvm_transactions_txdata, cvm_accounts SET id_from_addr = id WHERE from_addr = address;
UPDATE cvm_transactions_txdata, cvm_accounts SET id_to_addr = id WHERE to_addr = address;
ALTER TABLE cvm_transactions_txdata DROP INDEX cvm_transactions_txdata_from, DROP INDEX cvm_transactions_txdata_to;
ALTER TABLE cvm_transactions_txdata DROP COLUMN from_addr, DROP COLUMN to_addr;
CREATE INDEX ctx_id_from_addr on cvm_transactions_txdata(id_from_addr);
CREATE INDEX ctx_id_to_addr on cvm_transactions_txdata(id_to_addr);