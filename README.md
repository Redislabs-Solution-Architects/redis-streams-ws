# redis-streams-ws

An example using redis streams and web sockets to serve up data

## Run in Docker

```docker-compose up```

http://localhost:8080/

### Manually adding data to the stream

In RDBTools add to the stream named "stream" JSON similar to 
```
{"ric": "NYSE:UBER", price: "22.01"}
```

## Setup

1) Run a redis docker container or redis server version 5.0 or later
2) Run the Web server
```go run main.go```
3) navigate to http://localhost:8080/
4) throw a bunch of data in the stream
```for i in {1..2000} ; do  redis-cli  xadd stream  "*"  ric NYSE:UBER price 22.01 ; done```


## Building

```
docker build -t maguec/redis-streams-ws .
docker push  maguec/redis-streams-ws 
```
