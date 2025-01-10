#!/usr/bin/env bash

cleanup() {
    echo "Cleaning up..."
    if [[ -n "$provider_pid" ]]; then
        kill -SIGTERM "$provider_pid" 2>/dev/null
    fi
}

./cloudpipe provider &
provider_pid=$!
echo "Provider process started with PID $provider_pid"

trap cleanup EXIT

sleep 1

provider=http://localhost:8001

offer=`curl -s -u foo:bar $provider/backend/offers | jq -r .[0].name`
binding=`curl -s -X POST -u foo:bar $provider/backend/offers/$offer/bindings -H "Content-Type: application/json" -d '
{
    "id":"frontend"
}
'
`
pipe=`echo $binding | jq -r ._links.self.href`
eadapter=`curl -s -u foo:bar $provider/backend/offers/$offer | jq -r .defaultAdapters.[0]`
eproto=`curl -s -u foo:bar $provider/backend/offers/$offer/protos | jq -r .[0].id`
adapter=`echo $binding | jq -r .adapters.[0]`
proto=`echo $binding | jq -r .proto`

if [ "$eadapter" != "$adapter" ]; then
    echo "Adapters don't match: '$eadapter' != '$adapter'"
    exit 1
fi

if [ "$eproto" != "$proto" ]; then
    echo "Protos don't match: '$eproto' != '$proto'"
    exit 1
fi

eadapterhref=$provider/backend/offers/$offer/adapters/$eadapter
eprotohref=$provider/backend/offers/$offer/protos/$eproto
adapterhref=`curl -s -u foo:bar $pipe | jq -r ._links.adapters.[0].href`
protohref=`curl -s -u foo:bar $pipe | jq -r ._links.proto.href`

if [ "$eadapterhref" != "$adapterhref" ]; then
    echo "Adapter Hrefs don't match: '$eadapterhref' != '$adapterhref'"
    exit 1
fi

if [ "$eprotohref" != "$protohref" ]; then
    echo "Proto Hrefs don't match: '$eprotohref' != '$protohref'"
    exit 1
fi

euri=`curl -s -u foo:bar $pipe | jq -r .this.data.URI`

# test that uri can't be updated to an unmatching schema
code=`
curl -s --output /dev/stderr --write-out "%{http_code}" -X PATCH -u foo:bar $provider/backend/pipes/frontend -H "Content-Type: application/json" -d '
{
    "this": {
        "data":{
            "URI": "http://bad.data"
        }
    }
}
'
`
if [ "$code" != 400 ]; then
    echo "Code doesn't match $code != 400"
    exit 1
fi

uri=`curl -s -u foo:bar $pipe | jq -r .this.data.URI`


if [ "$euri" != "$uri" ]; then
    echo "URIs don't match: '$euri' != '$uri'"
    exit 1
fi

echo "SUCCESS"
