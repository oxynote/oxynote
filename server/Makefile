# Build the Docker image
run:
	make build --directory go
	make build --directory datagen
	make build --directory node
	docker-compose -p oxynote-server -f ./docker/docker-compose.yaml up
