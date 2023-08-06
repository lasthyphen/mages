alter table tx_pool drop column processed;

drop index tx_pool_msg_key on tx_pool;
drop index tx_pool_processed ON tx_pool;
drop index tx_pool_processed_created_at on tx_pool;

create index tx_pool_created_at on tx_pool (created_at asc);
create index tx_pool_topic on tx_pool (topic);