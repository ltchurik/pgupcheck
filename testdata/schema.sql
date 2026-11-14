--
-- PostgreSQL database dump (fixture for pgupcheck tests; mimics pg_dump 13 output)
--

SET statement_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;

CREATE EXTENSION IF NOT EXISTS adminpack WITH SCHEMA pg_catalog;

CREATE FUNCTION public.fact(bigint) RETURNS numeric
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT numeric_fac($1); $$;

CREATE OPERATOR public.!!! (
    FUNCTION = public.fact,
    LEFTARG = bigint
);

CREATE OPERATOR public.=== (
    FUNCTION = pg_catalog.texteq,
    LEFTARG = text,
    RIGHTARG = text
);

CREATE AGGREGATE public.array_accum(anyelement) (
    SFUNC = array_append,
    STYPE = anyarray,
    INITCOND = '{}'
);

CREATE FUNCTION public.norm_email(text) RETURNS text
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT lower(trim($1)); $$;

CREATE FUNCTION public.safe_norm(text) RETURNS text
    LANGUAGE sql IMMUTABLE
    SET search_path TO 'pg_catalog'
    AS $$ SELECT lower($1); $$;

CREATE FUNCTION public.take_backup() RETURNS void
    LANGUAGE plpgsql
    AS $_$
BEGIN
  -- a semicolon inside the body; must not split the statement
  PERFORM pg_start_backup('nightly');
END;
$_$;

CREATE FUNCTION public.py_hello() RETURNS text
    LANGUAGE plpythonu
    AS $$ return 'hi' $$;

CREATE FUNCTION public.py3_hello() RETURNS text
    LANGUAGE plpython3u
    AS $$ return 'hi' $$;

CREATE TABLE public.users (
    id bigint NOT NULL,
    email text
);

CREATE INDEX users_email_idx ON public.users USING btree (public.norm_email(email));

CREATE INDEX users_email_safe_idx ON public.users USING btree (public.safe_norm(email));

ALTER ROLE app SET password_encryption TO 'md5';

-- old_snapshot_threshold = 10 (in a comment; must not be reported)
