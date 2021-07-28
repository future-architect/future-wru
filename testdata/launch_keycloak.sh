#!/bin/bash

docker run -d --rm -p 18080:8080 --name keycloak \
-e KEYCLOAK_USER=admin -e KEYCLOAK_PASSWORD=admin \
 -e KEYCLOAK_IMPORT=/tmp/keycloak_wrusample_realm.json \
 -v $(pwd)/keycloak_wrusample_realm.json:/tmp/keycloak_wrusample_realm.json \
 wizzn/keycloak:14 # jboss/keycloak if you don't use M1 mac
