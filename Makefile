export HUB=hub.c.163.com/qingzhou/slime
lazyload:
	cd 	./slime-modules/lazyload && ./build.sh

limiter:
	cd 	./slime-modules/limiter && ./build.sh

plugin:
	cd 	./slime-modules/plugin && ./build.sh

slimeboot:
	cd ./slime-boot && ./build.sh

all: lazyload limiter plugin slimeboot