create index avm_outputs_transaction_id_asset_id_output_type on avm_outputs (transaction_id, asset_id, output_type);
create index avm_transactions_created_at_chain_id_idx on avm_transactions (created_at, chain_id);
drop index avm_transactions_created_at_idx on avm_transactions;

alter table pvm_blocks add column height bigint unsigned default 0 not null;