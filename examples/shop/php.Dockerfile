# One image for the shop, the receiver and the end-to-end test. The build context is
# the repository root, so the package is installed from this checkout.
FROM php:8.4-cli-alpine
RUN apk add --no-cache postgresql-libs \
    && apk add --no-cache --virtual .build postgresql-dev \
    && docker-php-ext-install pdo_pgsql \
    && apk del .build
COPY --from=composer:2 /usr/bin/composer /usr/bin/composer
COPY composer.json /package/composer.json
COPY src /package/src
COPY examples/shop/composer.json /app/composer.json
WORKDIR /app
RUN composer install --no-dev --no-progress --no-interaction
COPY examples/shop /app
