ALTER TABLE cvm_transactions_txdata ADD COLUMN from_addr varchar(50) AFTER id_from_addr, ADD COLUMN to_addr varchar(50) AFTER id_to_addr;
UPDATE cvm_transactions_txdata, cvm_accounts SET from_addr = address WHERE id_from_addr = id;
UPDATE cvm_transactions_txdata, cvm_accounts SET to_addr = address WHERE id_to_addr = id;

DROP TABLE cvm_accounts;

ALTER TABLE cvm_transactions_txdata DROP INDEX ctx_id_from_addr, DROP INDEX ctx_id_to_addr;
ALTER TABLE cvm_transactions_txdata DROP COLUMN id_from_addr, DROP COLUMN id_to_addr;
CREATE INDEX cvm_transactions_txdata_from on cvm_transactions_txdata(from_addr);
CREATE INDEX cvm_transactions_txdata_to on cvm_transactions_txdata(to_addr);
