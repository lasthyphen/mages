create table pvm_proposer
(
    id int not null auto_increment primary key,
    parent_id varchar(50),
    blk_id varchar(50),
    proposer_blk_id varchar(50),
    p_chain_height varchar(50),
    proposer varchar(50),
    time_stamp timestamp(6) default CURRENT_TIMESTAMP(6) not null,
    created_at timestamp(6) default CURRENT_TIMESTAMP(6) not null
);
create index pvm_proposer_blk_id ON pvm_proposer (blk_id);
create index pvm_proposer_proposer_blk_id ON pvm_proposer (proposer_blk_id);

INSERT INTO pvm_proposer (blk_id, p_chain_height, proposer, time_stamp, created_at)
  SELECT id, height, proposer, proposer_time, created_at
  FROM pvm_blocks;

ALTER TABLE pvm_blocks DROP COLUMN proposer, DROP COLUMN proposer_time;
ALTER TABLE cvm_blocks DROP COLUMN proposer, DROP COLUMN proposer_time;
