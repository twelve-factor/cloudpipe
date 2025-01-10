#!/usr/bin/env bash

cleanup() {
    echo "Cleaning up..."
    if [[ -n "$heroku_pid" ]]; then
        kill -SIGTERM "$heroku_pid" 2>/dev/null
    fi
}

./cloudpipe heroku &
heroku_pid=$!
echo "Heroku process started with PID $consumer_pid"

trap cleanup EXIT

sleep 1

heroku=http://localhost:8002

# create the backend pipe
curl -X POST -u foo:bar $heroku/cloudpipe-backend/pipes -H "Content-Type: application/json" -d '
{
    "id":"to_frontend"
}
'

curl -u foo:bar $heroku/cloudpipe-backend/pipes/to_frontend | jq .
# get the uri value from the pipe
uri=`curl -u foo:bar $heroku/cloudpipe-backend/pipes/to_frontend | jq -r .this.data.URI`


# create the frontend pipe
curl -X POST -u foo:bar $heroku/cloudpipe-frontend/pipes -H "Content-Type: application/json" -d "
{
    \"id\":\"to_backend\",
    \"other\": {\"data\":{\"URI\":\"$uri\"}}
}
"
curl -u foo:bar $heroku/cloudpipe-frontend/pipes/to_backend | jq .

