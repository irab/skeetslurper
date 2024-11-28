# Elastic Skeetslurper

## initialise submodules

```
git submodule update --init --recursive
```

## spin up ELK containers

```
cd elk
docker compose up setup
docker compose up -d
cd ..
```

## Run skeetslurper

```
go run .
```

or

```
go build
chmod +x skeetslurper
./skeetslurper
```


## Open Kibana and take a look

Open the Kibana URL in your browser:
http://localhost:5601/

Login with the following credentials:

Username: elastic
Password: changeme


Do not expose this outside of your local network, you will instantly get hacked.