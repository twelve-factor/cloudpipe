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

# create the db pipe
curl -X POST -u foo:bar $provider/db/pipes -H "Content-Type: application/json" -d '
{
    "id":"frontend"
}
'

curl -u foo:bar $provider/db/pipes/frontend | jq .
# get the uri value from the pipe
url=`curl -u foo:bar $provider/db/pipes/frontend | jq -r .this.data.URI`


# create the frontend pipe
curl -X POST -u foo:bar $consumer/frontend/pipes -H "Content-Type: application/json" -d "
{
    \"id\":\"db\",
    \"other\": {\"data\":{\"DATABASE_URL\":\"$url\"}}
}
"
curl -u foo:bar $consumer/frontend/pipes/db | jq .

