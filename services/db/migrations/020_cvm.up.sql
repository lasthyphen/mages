create table `cvm_blocks`
(
    block         decimal(65)      not null primary key,
    hash          varchar(100)     not null,
    chain_id      varchar(50)      not null,
    evm_tx        smallint         not null default 0,
    atomic_tx     smallint         not null default 0,
    serialization mediumblob,
    created_at    timestamp        not null default current_timestamp
);

create table `cvm_transactions_atomic`
(
    transaction_id varchar(50)     not null primary key,    
    block          decimal(65)     not null,
    chain_id       varchar(50)     not null,
    type           smallint        not null,
    created_at                     timestamp not null default current_timestamp
);

create table `cvm_addresses`
(
    id             varchar(50)     not null primary key,
    type           smallint        not null,
    idx            int unsigned    not null,
    transaction_id varchar(50)     not null,
    address        varchar(50)     not null,
    asset_id       varchar(50)     not null,
    amount         bigint unsigned not null default 0,
    nonce          bigint unsigned not null default 0,
    created_at     timestamp       not null default current_timestamp
);

create index cvm_blocks_hash ON cvm_blocks (hash);
create index cvm_address_transaction_id ON cvm_addresses (transaction_id);
