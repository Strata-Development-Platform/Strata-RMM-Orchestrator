#!/bin/bash
# Performance load test for Strata RMM platform
# Uses vegeta for HTTP load and custom agent simulator for NATS load
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
NATS_URL="${NATS_URL:-nats://localhost:4222}"
DURATION="${DURATION:-60s}"
RATE="${RATE:-100}"
AGENTS="${AGENTS:-500}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

check_deps() {
	for cmd in vegeta nats; do
		if ! command -v $cmd &>/dev/null; then
			log_error "Missing dependency: $cmd"
			exit 1
		fi
	done
}

test_api_health() {
	log_info "Testing API health endpoint..."
	echo "GET $API_URL/health" | vegeta attack -duration=10s -rate=50 \
		| vegeta report --type=text
}

test_api_metrics_query() {
	log_info "Testing metrics query endpoint..."
	echo "GET $API_URL/api/v1/metrics?tenant_id=00000000-0000-0000-0000-000000000001&metric_name=cpu_percent" \
		| vegeta attack -duration=$DURATION -rate=$RATE \
		| vegeta report --type=text
}

test_api_enrollment() {
	log_info "Testing enrollment endpoint..."
	echo "POST $API_URL/api/v1/enroll" \
		| vegeta attack -duration=$DURATION -rate=$RATE \
		-body '{"tenant_id":"00000000-0000-0000-0000-000000000001"}' \
		| vegeta report --type=text
}

simulate_agents() {
	log_info "Simulating $AGENTS agents publishing metrics via NATS..."

	cat > /tmp/agent-metrics.json <<'EOF'
{
  "tenant_id": "00000000-0000-0000-0000-000000000001",
  "device_id": "DEVICE_ID_PLACEHOLDER",
  "samples": [
    {"name": "cpu_percent", "value": 45.2, "tags": {"core": "0"}, "timestamp": TIMESTAMP},
    {"name": "mem_used_percent", "value": 62.1, "tags": {}, "timestamp": TIMESTAMP},
    {"name": "disk_used_percent", "value": 78.3, "tags": {"mount": "/"}, "timestamp": TIMESTAMP},
    {"name": "net_bytes_sent", "value": 1024000, "tags": {"iface": "eth0"}, "timestamp": TIMESTAMP},
    {"name": "net_bytes_recv", "value": 2048000, "tags": {"iface": "eth0"}, "timestamp": TIMESTAMP},
    {"name": "load_avg_1", "value": 1.5, "tags": {}, "timestamp": TIMESTAMP},
    {"name": "load_avg_5", "value": 1.2, "tags": {}, "timestamp": TIMESTAMP},
    {"name": "load_avg_15", "value": 1.0, "tags": {}, "timestamp": TIMESTAMP}
  ]
}
EOF

	START=$(date +%s)
	PUBLISHED=0
	END=$((START + $(echo $DURATION | sed 's/s//')))

	while [ $(date +%s) -lt $END ]; do
		for i in $(seq 1 $AGENTS); do
			DEVICE_ID="device-$(printf '%010d' $i)"
			TIMESTAMP=$(date +%s)000000000
			SUBJECT="tenant.00000000-0000-0000-0000-000000000001.agent.$DEVICE_ID.metrics"

			sed -e "s/DEVICE_ID_PLACEHOLDER/$DEVICE_ID/g" \
			    -e "s/TIMESTAMP/$TIMESTAMP/g" \
			    /tmp/agent-metrics.json | nats pub "$SUBJECT" > /dev/null 2>&1 &

			PUBLISHED=$((PUBLISHED + 1))
		done
		wait
		log_info "Published $PUBLISHED metric batches so far..."
		sleep 5
	done

	ELAPSED=$(( $(date +%s) - START ))
	log_info "Published $PUBLISHED metric batches in ${ELAPSED}s (avg: $((PUBLISHED / ELAPSED))/s)"
}

simulate_heartbeats() {
	log_info "Simulating heartbeats for $AGENTS agents..."
	for i in $(seq 1 $AGENTS); do
		DEVICE_ID="device-$(printf '%010d' $i)"
		SUBJECT="tenant.00000000-0000-0000-0000-000000000001.agent.$DEVICE_ID.heartbeat"
		PAYLOAD="{\"tenant_id\":\"00000000-0000-0000-0000-000000000001\",\"device_id\":\"$DEVICE_ID\",\"status\":\"online\",\"time\":$(date +%s)000000000,\"agent_version\":\"0.1.0\"}"
		echo "$PAYLOAD" | nats pub "$SUBJECT" > /dev/null 2>&1 &
	done
	wait
	log_info "Heartbeats published"
}

run_alert_storm() {
	log_info "Testing alert engine with metric threshold crossings..."

	for i in $(seq 1 100); do
		DEVICE_ID="device-$(printf '%010d' $i)"
		SUBJECT="tenant.00000000-0000-0000-0000-000000000001.agent.$DEVICE_ID.metrics"
		TIMESTAMP=$(date +%s)000000000
		CPU_VALUE=$(awk -v min=90 -v max=100 'BEGIN{srand(); print min+rand()*(max-min)}')

		PAYLOAD="{\"tenant_id\":\"00000000-0000-0000-0000-000000000001\",\"device_id\":\"$DEVICE_ID\",\"samples\":[{\"name\":\"cpu_percent\",\"value\":$CPU_VALUE,\"tags\":{},\"timestamp\":$TIMESTAMP}]}"
		echo "$PAYLOAD" | nats pub "$SUBJECT" > /dev/null 2>&1 &
	done
	wait
	log_info "Alert trigger metrics published"
}

run_full_suite() {
	log_info "========== Strata RMM Load Test Suite =========="
	log_info "API: $API_URL | NATS: $NATS_URL"
	log_info "Duration: $DURATION | Rate: $RATE | Agents: $AGENTS"
	echo ""

	test_api_health
	echo ""
	test_api_metrics_query
	echo ""
	test_api_enrollment
	echo ""
	simulate_heartbeats
	echo ""
	run_alert_storm
	echo ""
	simulate_agents

	log_info "========== Load test complete =========="
}

case "${1:-full}" in
	api)
		test_api_health
		test_api_metrics_query
		test_api_enrollment
		;;
	agents)
		simulate_heartbeats
		simulate_agents
		;;
	alerts)
		run_alert_storm
		;;
	full)
		run_full_suite
		;;
	*)
		echo "Usage: $0 [api|agents|alerts|full]"
		exit 1
		;;
esac
