#!/bin/bash
# Remove hardcoded clientID from git history
set -e

echo "Removing hardcoded clientID from git history..."
git filter-branch -f --tree-filter '
if [ -f cmd/agent/main.go ]; then
  sed -i "s/cid := \"2267d7c7-48ae-44fc-ab92-178beb5011f0\"/cid, err := config.Load(\"configs\/config.yaml\")/g" cmd/agent/main.go
  sed -i "/cid, err := config.Load/a \ \ \ \ if err != nil {\\n\ \ \ \ \ \ log.Fatalf(\"config load failed: %v\", err)\\n\ \ \ \ }" cmd/agent/main.go
fi
' -- --all

echo "Done! Hardcoded clientID has been removed from history."
echo "Run: git push --force-with-lease (if you need to force push to remote)"
