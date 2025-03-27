# cloudpipe: `heroku attach` for everything

## A Use Case

Meet Jasmine. Jasmine works for BigIT. She loves Heroku. She's tasked with building a an app to show cost-to-serve data for the business technology unit. The app is fairly straightforward, she just needs to pull data from the company's analytics database and display it for use by the exec team. This is just like many apps she's built before on Heroku: a web frontend and a database backend.

There's just one problem, the analytics database is using RDS. If she was connecting to a heroku database she would just use:

`heroku addons:attach -a frontend analytics-db`

And then she would refer to the database connection string in her code using:

`env['DATABASE_URL']`

Because the analytics database is not running on Heroku, she's forced to do the next best thing, which is to go to the AWS console, find the connection information for the database, and manually insert it as a config var for her app:

`heroku config:set -a frontend DATABASE_URL=postgres://...`

But then she remembers that AWS announced cloudpipe broker support for rds databases, which means she can use cloudpipe to get the config information instead:

```
result=`curl -X POST https://cloudpipe.aws.com/arn:rds:analytics-db/pipes -H "Content-Type: application/json" -d '
{
    "id":"frontend"
}
'`

url=`echo $result | jq -r .this.data.URI`

curl -X POST https://api.heroku.com/apps/frontend/pipes -H "Content-Type: application/json" -d "
{
    \"id\":\"database\",
    \"other\": {\"data\":{\"URL\":\"$url\"}}
}
"
```

Now there are a couple of things you may be thinking at this point:

1. That doesn't seem any easier than manually querying the data and adding it to config. To that I say: you're right! But you'll see the advantage soon.
2. This looks a whole lot like openservicebroker. Again, you're right. The binding step of openservicebroker looks a lot like the pipe creation step for the database. But cloudpipe also standardizes the mechanism for setting config on the other end of the connection, which is about to come in very handy.

Security at BigIT has an aggressive 30 day credential rotation policy. This means that every thirty days, Jasmine would have to manually repeat the same query and set process. She could automate this with a script, but she knows that the cloudpipe brokers will automatically update the config if she links the two ends of the pipe together:

```
result=`curl https://cloudpipe.aws.com/arn:rds:analytics-db/pipes/frontend`

uri=`echo $result | jq -r .this.uri`
issuer=`echo $result | jq -r .this.issuer`

curl -X PATCH https://api.heroku.com/apps/frontend/pipes/database -H "Content-Type: application/json" -d "
{
    \"other\": {\"uri\":\"$uri\", \"issuer\":\"$issuer\"}
}
"
result=`curl https://api.heroku.com/apps/frontend/pipes/database`

uri=`echo $result | jq -r .this.uri`
issuer=`echo $result | jq -r .this.issuer`

curl -X PATCH https://cloudpipe.aws.com/arn:rds:analytics-db/pipes/frontend -H "Content-Type: application/json" -d "
{
    \"other\": {\"uri\":\"$uri\", \"issuer\":\"$issuer\"}
}
"
```

Now any time the credentials for `anayltics-db` are updated, `frontend` will get a new config var just like it would with a `heroku addons:attach`!

## The Next Level

Things are going well for a few months, until BigIT co. has a security incident. Turns out a malicious actor managed to dump some databases using compromised credentials. In order to prevent future attacks, security rolled out a new policy enforcing that:
1. all databases must not be exposed to a the public internet (and)
2. all databases must use mtls

This would normally require a lot of complex manual setup, but Jasmine knows that cloudpipe supports special pipe blueprints to handle things like authentication, tunneling, and encryption. Instead of creating the pipe with the `/pipes` endpoint, she can create the pipe using the blueprints endpoints `/needs` and `/offers`. The `/needs` endpoint is exposed by the frontend broker, representing the services it needs to function (a database in this case), and the `/offers` endpoint is exposed by the database broker, representing the service it is offering. Here's what it looks like to bind to the blueprints:

```
curl -X POST https://cloudpipe.aws.com/arn:rds:analytics-db/offers/postgres/bindings -H "Content-Type: application/json" -d '
{
    "id":"frontend",
    "proto": "postgresqls",
    "adapters": ["conn:privateLink", "auth:mtls"],
    "other": {
        "uri":"...",
        "issuer":"..."
    }
}
'

curl -X POST https://api.heroku.com/apps/frontend/needs/db/bindings -H "Content-Type: application/json" -d '
{
    "id":"database",
    "proto": "postgresqls",
    "adapters": ["conn:privateLink", "auth:mtls"],
    "other": {
        "uri":"...",
        "issuer":"..."
    }
}
'
```

So Jasmine recreates the pipe using these endpoints and specifies two adapters. Because both brokers support the private link and mtls adapters, the brokers automatically set up the vpc peering and mtls certs necessary to allow the services to connect together. The only change Jasmine needs to make is to use the cert information in the newly provided config vars when making the connection to the database!

The proto and adapters fields of the blueprint binding also generate a schema that ensures that the sides agree on the necessary config data that must be supplied by each end of the pipe.

## A `cloudpipe`, Conceptually

Conceptually i think of cloudpipes as a progressive set of features that can be understood with an analogy to plumbing. The broker manages connections to the resource and advertises potential connections in the form of `blueprints`. The blueprint mechanism allows brokers to agree on the schema of the configuration data for the connection. This schema represents the size, shape, and material associated with the pipe. It doesn't make sense to connect a 20 inch pvc pipe to a 2 inch copper pipe. In the same way, a blueprint can ensure that the brokers are configuring their resources in a compatible way.

In order to connect to the resource, the user tells the broker to create each pipe. The pipe contains configuration information about connecting to and from the resource. The configuration can be thought of as the pipe material and the connection between resources is water flowing through the pipe.

The next conceptual leap involves connecting the two resources together by allowing brokers to share the configuration information with each other without needing an external user. This is equivalent to connecting the pipe-ends coming from each resource together.

Finally, the adapters extend the blueprint to support arbitrarily complex setup scenarios. I picture adapters as custom fittings on each end of the pipe. Individually, each part of the cloudpipe spec is simple, but it results in a scenario where connections can be automated. Services and application authors just need to worry about what they are connecting to, and the brokers handle the how.

## Other Uses

### Connecting to On-Prem Resources

Imagine you need to extend Jasmine's app to pull customer data exposed by a service that runs in a first-party datacenter. The service could implement a cloudpipe broker that supports automatic authentication of the frontend app using `auth:oidc`, and could even add a layer of protection by adding in ip whitelisting using a `conn:originIP` adapter. If BigCo runs their first-party service on kubernetes, then they would get this for free by installing a community supported cloudpipe broker for kuberentes.

### Unique adapters

One of the powers of the cloudpipe definition is the ability to introduce new types of configuration information that can become standardized. For example, LLM agents can use 'tools' which are custom functions exposed over http with metadata about how to call them, generally in the form of jsonschema. Unfortunately configuring your agent to use the tool requires different metadata and custom authentication depending on the provider. If we defined a new `meta:llm-tool` adapter standardizing the format, the heroku cloudpipe broker could expose it, and providing your tool to any llm would take the form of a cloudpipe binding using the new adapter. I could attach the same tool to openai, anthropic, meta, or bedrock using the same commands and even authenticate incoming commands using a standard adapter like `auth:oidc`.
