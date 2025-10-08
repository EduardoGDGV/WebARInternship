FROM wordpress:latest

WORKDIR /var/www/html

# Copy custom wp-config.php
COPY wp-config.php /var/www/html/wp-config.php
COPY .htaccess /var/www/html/.htaccess

# Copy all plugins and themes
COPY wp-content /var/www/html/wp-content
