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

# create the frontend pipe
curl -X POST -u foo:bar $heroku/cloudpipe-frontend/needs/backing_service/bindings -H "Content-Type: application/json" -d '
{
    "id":"to_backend",
    "proto": "https",
    "adapters": ["auth:oidc"],
    "other": {
        "uri":"http://localhost:8002/cloudpipe-backend/pipes/to_frontend",
        "issuer":"http://localhost:8002"
    }
}
'

# create the backend pipe
curl -X POST -u foo:bar $heroku/cloudpipe-backend/offers/backing_service/bindings -H "Content-Type: application/json" -d '
{
    "id":"to_frontend",
    "proto": "https",
    "adapters": ["auth:oidc"],
    "other": {
        "uri":"http://localhost:8002/cloudpipe-frontend/pipes/to_backend",
        "issuer":"http://localhost:8002"
    }
}
'
sleep 1
# view the pipes

curl -s -u foo:bar $heroku/cloudpipe-frontend/pipes/to_backend | jq '.this.data, .other.data'
curl -s -u foo:bar $heroku/cloudpipe-backend/pipes/to_frontend | jq '.this.data, .other.data'

euri=`curl -s -u foo:bar $heroku/cloudpipe-backend/pipes/to_frontend | jq -r .this.data.URI`
uri=`curl -s -u foo:bar $heroku/cloudpipe-frontend/pipes/to_backend | jq -r .other.data.URI`
if [ "$euri" != "$uri" ]; then
    echo "URIs don't match: '$euri' != '$uri'"
    exit 1
fi

euri=`heroku apps:info -a cloudpipe-backend | grep "Web URL" | awk '{print $3}'`
if [ "$euri" != "$uri" ]; then
    echo "URIs don't match: '$euri' != '$uri'"
    exit 1
fi
