# ForgottenWorld API 
Public facing API for the ForgottenWorld project. 

# API documentation

Get the list of available servers: https://api.example.com/servers
```
GET https://api.example.com/servers
```
```
 ["Creative","Mirias","Pixelmon"]
```
___
Get server status: https://api.example.com/server/{name}
```
GET https://api.example.com/server/Mirias
```
```
 {
   "online":30,
   "max":100
 }
```

# Running

Servers are defined in `servers.json`. See `servers.json.example`.
