# Slack Slash Command Handler

This will accept https GET/POST requests from slack and execute a plugin depending on the request.

Accepts API requests from Slack as part of a slash '/' command, which specify a plugin to execute + arguments to plugin
Below are the query parameters passed in from slack:

team_domain:[xyz]  
channel_name:[devopsbot-playground]   
user_name:[joseph]  
response_url:[xyz]   
text:[]    -- this is any argument they add after the command
script:[deploy.py]  
token:[xyz]   
team_id:[xyz]   
channel_id:[xyz]   
user_id:[xyz]   
command:[/deploy]]  

So, i'll be passing in these values as command line arguments to our plugins, and the plugin author can make use of any of them that they want.
