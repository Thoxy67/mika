/* create schema mika collate utf8mb4_unicode_ci; */

DROP TABLE IF EXISTS torrent;
create table torrent
(
    info_hash        binary(20)                     not null,
    total_uploaded   bigint unsigned   default 0    not null,
    total_downloaded bigint unsigned   default 0    not null,
    total_completed  smallint unsigned default 0    not null,
    is_deleted       tinyint(1)        default 0    not null,
    is_enabled       tinyint(1)        default 1    not null,
    reason           varchar(255)      default ''   not null,
    multi_up         decimal(5, 2)     default 1.00 not null,
    multi_dn         decimal(5, 2)     default 1.00 not null,
    seeders          int               default 0    not null,
    leechers         int               default 0    not null,
    announces        int               default 0    not null,
    constraint pk_torrent primary key (info_hash)
);

DROP TABLE IF EXISTS users;
create table users
(
    user_id          int unsigned auto_increment primary key,
    passkey          varchar(40)               not null,
    download_enabled tinyint(1)      default 1 not null,
    is_deleted       tinyint(1)      default 0 not null,
    downloaded       bigint unsigned default 0 not null,
    uploaded         bigint unsigned default 0 not null,
    announces        int             default 0 not null,
    constraint user_passkey_uindex unique (passkey)
);

DROP TABLE IF EXISTS peers;
create table peers
(
    peer_id          binary(20)                not null,
    info_hash        binary(20)                not null,
    user_id          int unsigned              not null,
    ipv6             boolean                   not null,
    addr_ip          int unsigned              not null,
    addr_port        smallint unsigned         not null,
    total_downloaded bigint unsigned default 0 not null,
    total_uploaded   bigint unsigned default 0 not null,
    total_left       bigint unsigned default 0 not null,
    total_time       int unsigned    default 0 not null,
    total_announces  int unsigned    default 0 not null,
    speed_up         int unsigned    default 0 not null,
    speed_dn         int unsigned    default 0 not null,
    speed_up_max     int unsigned    default 0 not null,
    speed_dn_max     int unsigned    default 0 not null,
    announce_first   datetime                  not null,
    announce_last    datetime                  not null,
    announce_prev    datetime                  not null,
    location         point                     not null,
    country_code     char(2)                   not null default '',
    asn              int unsigned              not null default 0,
    as_name          varchar(255)              not null default '',
    agent            varchar(100)              not null,
    crypto_level     int unsigned    default 0 not null,
    constraint peers_pk primary key (info_hash, peer_id)
);

DROP TABLE IF EXISTS whitelist;
create table whitelist
(
    client_prefix char(8)     not null primary key,
    client_name   varchar(20) not null
);


-- USERS
DROP PROCEDURE IF EXISTS user_by_passkey;
CREATE PROCEDURE user_by_passkey(IN in_passkey varchar(40))
BEGIN
    SELECT user_id,
           passkey,
           download_enabled,
           is_deleted,
           downloaded,
           uploaded,
           announces
    FROM users
    WHERE passkey = in_passkey;
end;

DROP PROCEDURE IF EXISTS `user_by_id`;
CREATE PROCEDURE user_by_id(IN in_user_id int)
BEGIN
    SELECT user_id,
           passkey,
           download_enabled,
           is_deleted,
           downloaded,
           uploaded,
           announces
    FROM users
    WHERE user_id = in_user_id;
end;

DROP PROCEDURE IF EXISTS user_delete;
CREATE PROCEDURE user_delete(IN in_user_id int)
BEGIN
    DELETE
    FROM users
    WHERE user_id = in_user_id;
end;

DROP PROCEDURE IF EXISTS user_add;
CREATE PROCEDURE user_add(IN in_user_id int,
                          IN in_passkey varchar(40),
                          IN in_download_enabled bool,
                          IN in_is_deleted bool,
                          IN in_downloaded bigint unsigned,
                          IN in_uploaded bigint unsigned,
                          IN in_announces bigint)
BEGIN
    INSERT INTO users
    (user_id, passkey, download_enabled, is_deleted, downloaded, uploaded, announces)
    VALUES (in_user_id, in_passkey, in_download_enabled, in_is_deleted,
            in_downloaded, in_uploaded, in_announces);
end;

DROP PROCEDURE IF EXISTS user_update;
CREATE PROCEDURE user_update(IN in_user_id int,
                             IN in_passkey varchar(40),
                             IN in_download_enabled bool,
                             IN in_is_deleted bool,
                             IN in_downloaded bigint unsigned,
                             IN in_uploaded bigint unsigned,
                             IN in_announces bigint,
                             IN in_old_passkey varchar(40))
BEGIN
    UPDATE users
    SET user_id          = in_user_id,
        passkey          = in_passkey,
        download_enabled = in_download_enabled,
        is_deleted       = in_is_deleted,
        downloaded       = in_downloaded,
        uploaded         = in_uploaded,
        announces        = in_announces
    WHERE passkey = if(in_old_passkey = '', in_passkey, in_old_passkey);
end;

DROP PROCEDURE IF EXISTS user_update_stats;
CREATE PROCEDURE user_update_stats(IN in_passkey varchar(40),
                                   IN in_announces bigint,
                                   IN in_uploaded bigint,
                                   IN in_downloaded bigint)
BEGIN
    UPDATE users
    SET announces  = (announces + in_announces),
        uploaded   = (uploaded + in_uploaded),
        downloaded = (downloaded + in_downloaded)
    WHERE passkey = in_passkey;
END;

-- END USERS

-- TORRENTS

DROP PROCEDURE IF EXISTS torrent_by_infohash;
CREATE PROCEDURE torrent_by_infohash(IN in_info_hash binary(20),
                                     IN in_deleted bool)
BEGIN
    SELECT info_hash,
           total_uploaded,
           total_downloaded,
           total_completed,
           is_deleted,
           is_enabled,
           reason,
           multi_up,
           multi_dn,
           seeders,
           leechers,
           announces
    FROM torrent
    WHERE info_hash = in_info_hash
      AND is_deleted = in_deleted;
end;

DROP PROCEDURE IF EXISTS torrent_delete;
CREATE PROCEDURE torrent_delete(IN in_info_hash binary(20))
BEGIN
    DELETE
    FROM torrent
    WHERE info_hash = in_info_hash;
end;

DROP PROCEDURE IF EXISTS torrent_disable;
CREATE PROCEDURE torrent_disable(IN in_info_hash binary(20))
BEGIN
    UPDATE torrent
    SET is_deleted = true
    WHERE info_hash = in_info_hash;
end;

DROP PROCEDURE IF EXISTS torrent_add;
CREATE PROCEDURE torrent_add(IN in_info_hash binary(20))
BEGIN
    INSERT INTO torrent (info_hash)
    VALUES (in_info_hash);
end;

DROP PROCEDURE IF EXISTS torrent_update_stats;
CREATE PROCEDURE torrent_update_stats(IN in_info_hash binary(20),
                                      IN in_total_downloaded bigint unsigned,
                                      IN in_total_uploaded bigint unsigned,
                                      IN in_announces bigint,
                                      IN in_total_completed int,
                                      IN in_seeders int,
                                      IN in_leechers int)
BEGIN
    UPDATE
        torrent
    SET total_downloaded = (total_downloaded + in_total_downloaded),
        total_uploaded   = (total_uploaded + in_total_uploaded),
        announces        = (announces + in_announces),
        total_completed  = (total_completed + in_total_completed),
        seeders          = in_seeders,
        leechers         = in_leechers
    WHERE info_hash = in_info_hash;
END;

DROP PROCEDURE IF EXISTS whitelist_all;
CREATE PROCEDURE whitelist_all()
BEGIN
    SELECT *
    FROM whitelist;
end;

DROP PROCEDURE IF EXISTS whitelist_add;
CREATE PROCEDURE whitelist_add(IN in_client_prefix char(5),
                               IN in_client_name varchar(255))
BEGIN
    INSERT INTO whitelist (client_prefix, client_name)
    VALUES (in_client_prefix, in_client_name);
end;

DROP PROCEDURE IF EXISTS whitelist_delete_by_prefix;
CREATE PROCEDURE whitelist_delete_by_prefix(IN in_client_prefix varchar(255))
BEGIN
    DELETE
    FROM whitelist
    WHERE client_prefix = in_client_prefix;
end;

-- END TORRENTS

-- PEERS
DROP PROCEDURE IF EXISTS peer_update_stats;
CREATE PROCEDURE peer_update_stats(IN in_info_hash binary(20),
                                   IN in_peer_id binary(20),
                                   IN in_total_downloaded bigint unsigned,
                                   IN in_total_uploaded bigint unsigned,
                                   IN in_total_announces bigint,
                                   IN in_announce_last datetime,
                                   IN in_speed_dn bigint,
                                   IN in_speed_up bigint,
                                   IN in_speed_dn_max bigint,
                                   IN in_speed_up_max bigint)
BEGIN
    UPDATE
        peers
    SET total_announces  = (total_announces + in_total_announces),
        total_downloaded = (total_downloaded + in_total_downloaded),
        total_uploaded   = (total_uploaded + in_total_uploaded),
        announce_last    = in_announce_last,
        speed_up         = in_speed_up,
        speed_dn         = in_speed_dn,
        speed_up_max     = GREATEST(speed_up_max, in_speed_up_max),
        speed_dn_max     = GREATEST(speed_dn_max, in_speed_dn_max)

    WHERE info_hash = in_info_hash
      AND peer_id = in_peer_id;
END;

DROP PROCEDURE IF EXISTS peer_reap;
CREATE PROCEDURE peer_reap(IN in_expiry_time datetime)
BEGIN
    DELETE
    FROM peers
    WHERE announce_last <= in_expiry_time;
end;

DROP PROCEDURE IF EXISTS peer_add;
CREATE PROCEDURE peer_add(IN in_info_hash binary(20),
                          IN in_peer_id binary(20),
                          IN in_user_id int,
                          IN in_ipv6 boolean,
                          IN in_addr_ip varchar(255),
                          IN in_addr_port int,
                          IN in_location varchar(255),
                          IN in_announce_first datetime,
                          IN in_announce_last datetime,
                          IN in_downloaded bigint unsigned,
                          IN in_uploaded bigint unsigned,
                          IN in_left bigint,
                          IN in_client varchar(255),
                          IN in_country_code char(2),
                          IN in_asn varchar(10),
                          IN in_as_name varchar(255),
                          IN in_crypto_level int)
BEGIN
    INSERT INTO peers
    (peer_id, info_hash, user_id, ipv6, addr_ip, addr_port, location, announce_first, announce_last, announce_prev,
     total_downloaded, total_uploaded, total_left, agent, country_code, asn, as_name, crypto_level)
    VALUES (in_peer_id,
            in_info_hash,
            in_user_id,
            in_ipv6,
            if(in_ipv6 = false, INET_ATON(in_addr_ip), INET6_ATON(in_addr_ip)),
            in_addr_port,
            ST_PointFromText(in_location),
            in_announce_first,
            in_announce_last,
            in_announce_last,
            in_downloaded,
            in_uploaded,
            in_left,
            in_client,
            in_country_code,
            in_asn,
            in_as_name,
            in_crypto_level);
end;

DROP PROCEDURE IF EXISTS peer_delete;
CREATE PROCEDURE peer_delete(IN in_info_hash binary(20),
                             IN in_peer_id binary(20))
BEGIN
    DELETE
    FROM peers
    WHERE info_hash = in_info_hash
      AND peer_id = in_peer_id;
end;

DROP PROCEDURE IF EXISTS peer_get;
CREATE PROCEDURE peer_get(IN in_info_hash binary(20), IN in_peer_id binary(20))
BEGIN
    SELECT peer_id,
           info_hash,
           user_id,
           ipv6,
           if(ipv6 = false, INET_NTOA(addr_ip), INET6_NTOA(addr_ip)) as addr_ip,
           addr_port,
           total_downloaded,
           total_uploaded,
           total_left,
           total_time,
           total_announces,
           speed_up,
           speed_dn,
           speed_up_max,
           speed_dn_max,
           ST_AsText(location)                                       as location,
           announce_last,
           announce_first,
           country_code,
           asn,
           as_name,
           crypto_level                                              as crypto_level
    FROM peers
    WHERE info_hash = in_info_hash
      AND peer_id = in_peer_id;
end;

DROP PROCEDURE IF EXISTS peer_get_n;
CREATE PROCEDURE peer_get_n(IN in_info_hash binary(20), IN in_limit int)
BEGIN
    SELECT peer_id,
           info_hash,
           user_id,
           ipv6,
           if(ipv6 = false, INET_NTOA(addr_ip), INET6_NTOA(addr_ip)) as addr_ip,
           addr_port,
           total_downloaded,
           total_uploaded,
           total_left,
           total_time,
           total_announces,
           speed_up,
           speed_dn,
           speed_up_max,
           speed_dn_max,
           ST_AsText(location)                                       as location,
           announce_last,
           announce_first,
           country_code,
           asn,
           as_name,
           crypto_level                                              as crypto_level
    FROM peers
    WHERE info_hash = in_info_hash
    LIMIT in_limit;
end;
-- END PEERS