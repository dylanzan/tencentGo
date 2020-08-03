#!/usr/bin bash

cd /root/.pm2/logs/
logsHour=`ls -l |awk -F "_" '{print $4}' |awk -F "." '{print $1}'`
logsDay= `ls -al |awk -F "__" '{print $2}' |awk -F "-" '{print $3}' |awk -F "_" '{print $1}'`

currentDay=`date "+%D" |awk -F "/" '{print $2}'`
currentHour=`date "+%H"`

for i in `ls -al |awk '{print $9}' |grep tracking`
do
  if [ $logsDay -le $currentDay ]
  then
    rm -rvf i
  fi
  if [ $logsDay -eq $currentDay ]
  then
    if [ $logsHour -le $currentHour-2 ]
    then
      rm -rvf i
    fi
  fi
done


