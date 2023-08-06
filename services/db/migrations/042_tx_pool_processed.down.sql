alter table tx_pool add processed smallint unsigned not null default 0 AFTER serialization;

create index tx_pool_msg_key on tx_pool (msg_key);
create index tx_pool_processed ON tx_pool (topic, processed, created_at asc);
create index tx_pool_processed_created_at on tx_pool (processed, created_at asc);

drop index tx_pool_created_at on tx_pool;
drop index tx_pool_topic on tx_pool;