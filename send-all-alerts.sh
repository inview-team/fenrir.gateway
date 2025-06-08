#!/bin/bash

# This script sends test alerts for multiple deployments.

DEPLOYMENTS=("frontend" "redis-leader" "redis-follower")

for deployment in "${DEPLOYMENTS[@]}"
do
  echo "--- Sending alert for ${deployment} ---"
  ./send-test-alert.sh "${deployment}"
  echo "--------------------------------------"
  echo ""
  sleep 1
done

echo "All test alerts sent."
