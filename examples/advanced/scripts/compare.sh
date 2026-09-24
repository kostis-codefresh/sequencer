#!/bin/sh
set -eu

for f in argo-rollouts.zip argo-rollouts.tar.gz argo-rollouts.tar.bz2; do
  stat --format='%s %n' "$f"
done | sort -n | cut -d' ' -f2 > comparison.txt
