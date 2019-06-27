# redis-streams-ws

An example using redis streams and web sockets to serve up data

## Setup

1) Run a redis docker container or redis server version 5.0 or later
2) Run the Web server
```go run main.go```
3) navigate to http://localhost:8080/
4) throw a bunch of data in the stream
```for i in {1..3000} ; do  redis-cli  xadd stream  "*" foo${i} bar; done```

