ALTER TABLE pvm_blocks ADD COLUMN (proposer varchar(50), proposer_time timestamp(6) NULL);
ALTER TABLE cvm_blocks ADD COLUMN (proposer varchar(50), proposer_time timestamp(6) NULL);

UPDATE pvm_blocks, pvm_proposer
SET pvm_blocks.proposer = pvm_proposer.proposer, 
    pvm_blocks.proposer_time = pvm_proposer.time_stamp 
WHERE 
    pvm_proposer.blk_id = pvm_blocks.id;

drop table pvm_proposer;