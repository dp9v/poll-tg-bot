Develop a Telegram bot on golang. 
The bot should every 5 minutes send request to URL:
    https://n1338118.alteg.io/api/v1/activity/777474/search with parameters:
    from: current date in a format: YYYY-MM-DD
    till: 1st day of the next month + 2 months in a format: YYYY-MM-DD
    service_ids[]=12995896
    staff_ids[]=2811495 

### Business logic:
The example of the answer can be found in the file: requirements/search-response-example.json

1) After receiving the response need to keep only available activities: where "capacity" value more that "records_count"
2) If the list of avaialble activities changed since the last request, the bot should send a message to 
3) Telegram specific user with the list of available dates. The message should contain the following information about each activity:
   - date
   - time
   - title
   - staff.name (Coach name)
   - number of available places (capacity - records_count)
   - link to the activity (https://n1338118.alteg.io/company/777474/activity/info/9918716)
4) Need to send update only if new activities appeared or disappeared since the last request. If the list of available activities is the same as in the previous request, no message should be sent.
5) The bot should run continuously and send updates every 5 minutes.

### Technical requirements:
1) The bot should be developed in Golang.
2) BOT_TOKEN AND USER_ID/CHAT_ID should be stored in environment variables.
3) To send a request Bear token should be used for authorization. The token can be obtained from the Alteg platform(https://n1338118.alteg.io/company/777474/about).
4) Intermediate data (list of available activities) can be stored in memory, no need for a database.
5) The bot should handle errors gracefully, for example, if the request to the API fails and send a short message to the chat about the error.
6) Error message should be send once in a row, if the error persists, no need to send the same message multiple times. Only when the error is resolved and then appears again, the message should be sent again.
7) Result artifact should be a Docker image that can be run from docker-compose. The docker-compose file should be included in the project. The image should be built using a Dockerfile, which should also be included in the project. The Dockerfile should use a multi-stage build to optimize the image size. The final image should be as small as possible while still containing all necessary dependencies to run the bot.