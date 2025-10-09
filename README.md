# Nakama Backend (+Basic Frontend Example)

This folder initiates a Nakama environment (using a preexisting Docker setup), with postgres, go server files and a basic javascript frontend.

## Commands:

1. Start the example frontend server

```
    cd nakama/frontend/src/
    npm run dev
```

>_Access it through http://127.0.0.1:5173/_

2. Test bots (simulate players) after Signup/Login

```
    node ./bots.js
```

>On another terminal at nakama/frontend/src/, with the server running

## Relevant links

* Frontend example: http://localhost:5173/
* Nakama console: http://localhost:7351/

>**TO BE CHANGED!**
>
>```
>login: admin
>password: password
>```