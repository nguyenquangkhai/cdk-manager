#!/usr/bin/env bash
# Fake cdk for tests. Behavior controlled by env FAKE_MODE.
echo "synthesizing CloudFormation template"
echo "deploying $*"
if [ "$FAKE_MODE" = "fail" ]; then
  echo "error: boom" >&2
  exit 1
fi
echo "done"
exit 0
