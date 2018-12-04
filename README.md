# clanforge-game-coordinator
A server allocator and player load balancer for Clanforge using Steam authentication

### Documentation

App Engine for Go Docs: https://cloud.google.com/appengine/docs/standard/go/

Clanforge Docs: https://docs.multiplay.com/display/CF/API+Reference+Documentation

Steam Auth Docs: https://partner.steamgames.com/doc/features/auth

### Requirements

There are several required keys created by Clanforge and Steam that need files created in the coordinator directory. Each file contains a single line with the key. These paths are constants that can be changed in api-clanforge.go and api-steam.go:
- clanforge-api-access.key // Put your Clanforge Access Key in here
- clanforge-api-secret.key // Put your Clanforge Secret key in here
- clanforge-api-profile.key // Put the Clanforge profile you are using for your deployment
- clanforge-api-region-na.key // Put a Clanforge region (in this case na) you are using for your deployment
- clanforge-api-region-eu.key // Put an alternative Clanforge region (in this case eu) you are using for your deployment
- steam-api.key // Put your Steam publisher key in here: https://partner.steamgames.com/doc/webapi_overview/auth
- steam-appid.key // Put your game's appid in here

### Known Issues

When installing and/or deploying to App Engine with gcloud, there may be issues (import cycles, missing packages, failed deployment) with AWS request signing to do with JMESPath. As this coordinator only uses request signing from the SDK, the solution I took was to remove the references directly within the imported AWS package in my GOPATH. Hopefully this will be solved in later versions of the AWS SDK.
