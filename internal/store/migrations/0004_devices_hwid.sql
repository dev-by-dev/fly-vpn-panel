-- Устройство теперь опознаётся по HWID клиента, а когда его нет —
-- по IP. Старый UNIQUE(username, ip) склеивал устройства за одним NAT.
CREATE TABLE devices_new (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL,
	ip TEXT NOT NULL,
	first_seen TEXT NOT NULL,
	last_seen TEXT NOT NULL,
	country TEXT NOT NULL DEFAULT '',
	asn TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	hwid TEXT NOT NULL DEFAULT '',
	platform TEXT NOT NULL DEFAULT '',
	os_version TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT ''
);
INSERT INTO devices_new (id, username, ip, first_seen, last_seen, country, asn, provider, hwid, platform, os_version, model, user_agent)
	SELECT id, username, ip, first_seen, last_seen, country, asn, provider, hwid, platform, os_version, model, user_agent FROM devices;
DROP TABLE devices;
ALTER TABLE devices_new RENAME TO devices;
CREATE UNIQUE INDEX devices_hwid ON devices(username, hwid) WHERE hwid <> '';
CREATE UNIQUE INDEX devices_ip ON devices(username, ip) WHERE hwid = '';
