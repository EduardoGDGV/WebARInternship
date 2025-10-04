# _Polimon_

This folder initiates a Nakama + Wordpress Docker setup, with a built in frontend (nakama/frontend).

## Commands:

>Start from the root!
>
>```
>cd main/
>```

1. Start/End docker

```
docker compose up -d --build
```

>May take a couple of minutes...

```
docker compose down
```

>Add -v to remove previous volumes

2. Monitor real-time Nakama and Wordpress logs

```
docker compose logs -f nakama
```

```
docker compose logs -f wordpress
```

3. Monitor containers' status

```
docker ps
```

## Relevant links

* Frontend example: http://localhost:5173/
* Nakama console: http://localhost:7351/

>**TO BE CHANGED!**
>
>```
>login: admin
>password: password
>```

* Wordpress admin page: http://localhost:8081/wp-admin/

>**ON FIRST BUILD**
>
>1. Access the wordpress page: http://localhost:8081
>2. Select language, install wordpress, set page info and credentials
>3. Following builds will include the wordpress build, unless removed (to be changed!)