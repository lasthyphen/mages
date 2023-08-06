create table `cvm_transactions_receipts`
(
    hash           varchar(100)    not null,
    status         smallInt        unsigned not null default 0,
    gas_used       bigInt          unsigned not null default 0,
    serialization  mediumblob,
    created_at     timestamp(6)    not null default current_timestamp(6),
    primary key(hash)
);
