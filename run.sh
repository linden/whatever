#!/bin/bash

cd /app

./main | tee main.log &
 
caddy run | tee caddy.log &

wait -n

exit $?