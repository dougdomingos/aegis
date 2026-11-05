#!/usr/bin/env bash
# Builds the real binaries and installs every service on the running VMs.
source "$(dirname "$0")/lib.sh"

log "Building binaries"
(cd "$ROOT_DIR" && make -s proto &&
	CGO_ENABLED=0 go build -buildvcs=false -o "$STATE_DIR/aegis-orchestrator" ./cmd/orchestrator &&
	CGO_ENABLED=0 go build -buildvcs=false -o "$STATE_DIR/aegis-sentinel" ./cmd/sentinel)

(umask 077; openssl rand -hex 32 >"$STATE_DIR/lab.key")
session_secret="$(openssl rand -hex 32)"

# The DoH certificate is self-signed, valid for the services VM address only.
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj /CN=aegis-doh \
	-keyout "$STATE_DIR/doh.key" -out "$STATE_DIR/doh.pem" \
	-addext "subjectAltName=IP:$SERVICES_IP" 2>/dev/null

# Services first: the Sentinel needs the upstream DNS and the Orchestrator.
log "Deploying the services VM"
vm_scp "$SERVICES_IP" "$TEMPLATES/dnsmasq.conf" dnsmasq.conf
vm_scp "$SERVICES_IP" "$TEMPLATES/aegis-http@.service" aegis-http@.service
for file in "$STATE_DIR/doh.pem" "$STATE_DIR/doh.key" "$TEMPLATES/doh-server.py" "$TEMPLATES/aegis-doh.service"; do
	vm_scp "$SERVICES_IP" "$file" "$(basename "$file")"
done
vm_ssh "$SERVICES_IP" 'sudo bash -es' <<'REMOTE'
install -m 644 dnsmasq.conf /etc/dnsmasq.d/aegis.conf
install -m 644 aegis-http@.service /etc/systemd/system/
mkdir -p /var/www/aegis && echo aegis-target >/var/www/aegis/index.html
mkdir -p /etc/aegis-doh
install -m 644 doh.pem doh-server.py /etc/aegis-doh/
install -m 600 doh.key /etc/aegis-doh/doh.key
install -m 644 aegis-doh.service /etc/systemd/system/
rm -f doh.pem doh.key doh-server.py aegis-doh.service
systemctl daemon-reload
systemctl enable dnsmasq aegis-doh aegis-http@192.168.150.31 aegis-http@192.168.150.32
systemctl restart dnsmasq aegis-doh aegis-http@192.168.150.31 aegis-http@192.168.150.32
touch /etc/cloud/cloud-init.disabled
REMOTE

log "Deploying the Orchestrator VM"
for file in "$STATE_DIR/aegis-orchestrator" "$STATE_DIR/lab.key" \
	"$TEMPLATES/machines.csv" "$TEMPLATES/aegis-orchestrator.service"; do
	vm_scp "$ORCHESTRATOR_IP" "$file" "$(basename "$file")"
done
vm_ssh "$ORCHESTRATOR_IP" "sudo SESSION_SECRET=$session_secret bash -es" <<'REMOTE'
mkdir -p /etc/aegis /var/lib/aegis
install -m 755 aegis-orchestrator /usr/local/bin/
install -m 600 lab.key /etc/aegis/lab.key
install -m 644 machines.csv /etc/aegis/machines.csv
install -m 644 aegis-orchestrator.service /etc/systemd/system/
(umask 077; cat >/etc/aegis/orchestrator.env <<ENV
AEGIS_LAB_KEY=/etc/aegis/lab.key
AEGIS_SESSION_SECRET=$SESSION_SECRET
AEGIS_MACHINES=/etc/aegis/machines.csv
AEGIS_DB=/var/lib/aegis/aegis.db
AEGIS_GRPC_ADDR=:9090
AEGIS_STALENESS_GRACE=15s
ENV
)
rm -f lab.key
systemctl daemon-reload
systemctl enable --now aegis-orchestrator
touch /etc/cloud/cloud-init.disabled
REMOTE

log "Deploying the Sentinel VM"
for file in "$STATE_DIR/aegis-sentinel" "$STATE_DIR/lab.key" \
	"$TEMPLATES/sentinel.config.json" "$TEMPLATES/aegis-sentinel.service" \
	"$STATE_DIR/doh.pem" "$TEMPLATES/chrome-doh-policy.json"; do
	vm_scp "$SENTINEL_IP" "$file" "$(basename "$file")"
done
vm_ssh "$SENTINEL_IP" 'sudo bash -es' <<'REMOTE'
mkdir -p /etc/aegis /var/lib/aegis
install -m 755 aegis-sentinel /usr/local/bin/
install -m 600 lab.key /etc/aegis/lab.key
install -m 644 sentinel.config.json /etc/aegis/sentinel.json
install -m 644 aegis-sentinel.service /etc/systemd/system/
rm -f lab.key
id student >/dev/null 2>&1 || useradd -m student
install -m 644 doh.pem chrome-doh-policy.json /etc/aegis/
rm -f doh.pem chrome-doh-policy.json
# Chrome trusts certificates through the user's NSS database.
sudo -u student -H sh -c 'mkdir -p "$HOME/.pki/nssdb" &&
	certutil -d "sql:$HOME/.pki/nssdb" -N --empty-password &&
	certutil -d "sql:$HOME/.pki/nssdb" -A -t "C,," -n aegis-doh -i /etc/aegis/doh.pem'
systemctl disable --now systemd-resolved
rm -f /etc/resolv.conf
echo 'nameserver 127.0.0.1' >/etc/resolv.conf
systemctl daemon-reload
systemctl enable --now aegis-sentinel
touch /etc/cloud/cloud-init.disabled
REMOTE
