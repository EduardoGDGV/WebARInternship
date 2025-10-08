# Wordpress Backend

This folder initiates a Wordpress environment (using a preexisting Docker setup), with sql and custom plugins.

## Relevant files/folders
* .htaccess
* wp-config.php
* wp-content/

## Relevant links

* Wordpress admin page: http://localhost:8081/wp-admin/
* Wordpress json posts page: http://localhost:8081/wp-json/wp/v2/posts/

>**ON FIRST BUILD**
>
>1. Access the wordpress page: http://localhost:8081
>2. Select language, install wordpress, set page info and credentials
>3. Following builds will include the wordpress build, unless removed (to be changed!)