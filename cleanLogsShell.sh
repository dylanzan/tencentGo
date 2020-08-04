#!/usr/bin bash

cd /root/.pm2/logs/

currentDay=`date "+%D" |awk -F "/" '{print $2}' |awk '{print int($0)}'`
currentHour=`date "+%H" |awk '{print int($0)}'`

for i in `ls -al |awk '{print $9}' |grep tencentGo`
do
  echo "$i"
  logsHour=`echo "$i" |awk -F "_" '{print $4}' |awk -F "." '{print $1}' |awk '{print int($0)}'`
  logsDay=`echo "$i" |awk -F "__" '{print $2}' |awk -F "-" '{print $3}' |awk -F "_" '{print $1}' |awk '{print int($0)}'`
  if [ "$logsHour" -eq 0 ]
  then
    continue
  fi
  # shellcheck disable=SC2122
  if [ "$logsDay" -lt "$currentDay" ]
  then
    rm -rvf `echo $i`
  fi
  if [ "$logsDay" -eq "$currentDay" ]
  then
    # shellcheck disable=SC2122
    if [ "$logsHour" -lt "$currentHour" ]
    then
      rm -rvf `echo $i`
    fi
  fi
done
