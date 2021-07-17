mkdir -p /var/log/ally

export SLACK_BOT_TOKEN=xoxb-foo
export SLACK_APP_TOKEN=xapp-bar
export CODEFRESH_API_KEY=bubu

nohup ally ally.toml >> /var/log/ally/out.log &
