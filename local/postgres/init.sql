CREATE DATABASE ngo_db;
CREATE DATABASE donation_db;

\connect ngo_db
\i /sql/ngo.sql

\connect donation_db
\i /sql/donation.sql
