-- Create replication user
CREATE USER replicator REPLICATION LOGIN CONNECTION LIMIT 5 ENCRYPTED PASSWORD 'replicator_password';

-- Grant necessary permissions
GRANT CONNECT ON DATABASE postgres TO replicator;

-- POC

CREATE TABLE tbl_users (
    id UUID NOT NULL,
    first_name VARCHAR(255) NOT NULL,
    PRIMARY KEY (id)
) PARTITION BY HASH (id);

-- Create 4 partitions
CREATE TABLE tbl_users_p0 PARTITION OF tbl_users
    FOR VALUES WITH (MODULUS 4, REMAINDER 0);

CREATE TABLE tbl_users_p1 PARTITION OF tbl_users
    FOR VALUES WITH (MODULUS 4, REMAINDER 1);

CREATE TABLE tbl_users_p2 PARTITION OF tbl_users
    FOR VALUES WITH (MODULUS 4, REMAINDER 2);

CREATE TABLE tbl_users_p3 PARTITION OF tbl_users
    FOR VALUES WITH (MODULUS 4, REMAINDER 3);