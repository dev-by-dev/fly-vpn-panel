-- Клиенты без HWID различаются парой IP + User-Agent: иначе два разных
-- клиента за одним NAT сливаются в одну строку.
DROP INDEX IF EXISTS devices_ip;
CREATE UNIQUE INDEX devices_ip_ua ON devices(username, ip, user_agent) WHERE hwid = '';
