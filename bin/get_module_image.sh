
curl 'https://registry.hub.docker.com/v2/repositories/slimeio/slime-${MOD}/tags?page_size=${PAGE_SIZE:-1024}' | jq -r '."results"[]["name"]'
