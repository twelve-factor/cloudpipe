#!/usr/bin/env bash

cleanup() {
    echo "Cleaning up..."
    if [[ -n "$consumer_pid" ]]; then
        kill -SIGTERM "$consumer_pid" 2>/dev/null
    fi
    if [[ -n "$provider_pid" ]]; then
        kill -SIGTERM "$provider_pid" 2>/dev/null
    fi
}

./cloudpipe consumer &
consumer_pid=$!
echo "Consumer process started with PID $consumer_pid"

./cloudpipe provider &
provider_pid=$!
echo "Provider process started with PID $provider_pid"

trap cleanup EXIT

sleep 1

consumer=http://localhost:8000
provider=http://localhost:8001

# create the frontend pipe
curl -X POST -u foo:bar $consumer/frontend/needs/backend/bindings -H "Content-Type: application/json" -d '
{
    "id":"backend",
    "proto": "https",
    "adapters": ["auth:oidc"],
    "other": {
        "uri":"http://localhost:8001/backend/pipes/frontend",
        "issuer":"http://localhost:8001"
    }
}
'

# create the backend pipe
curl -X POST -u foo:bar $provider/backend/offers/https/bindings -H "Content-Type: application/json" -d '
{
    "id":"frontend",
    "proto": "https",
    "adapters": ["auth:oidc"],
    "other": {
        "uri":"http://localhost:8000/frontend/pipes/backend",
        "issuer":"http://localhost:8000"
    }
}
'
sleep 1
# view the pipes

curl -s -u foo:bar $consumer/frontend/pipes/backend | jq '.this.data, .other.data'
curl -s -u foo:bar $provider/backend/pipes/frontend | jq '.this.data, .other.data'

euri=`curl -s -u foo:bar $provider/backend/pipes/frontend | jq -r .this.data.URI`
uri=`curl -s -u foo:bar $consumer/frontend/pipes/backend | jq -r .other.data.URI`
if [ "$euri" != "$uri" ]; then
    echo "URIs don't match: '$euri' != '$uri'"
    exit 1
fi

euri=https://backend.herokuapp.com
if [ "$euri" != "$uri" ]; then
    echo "URIs don't match: '$euri' != '$uri'"
    exit 1
fi