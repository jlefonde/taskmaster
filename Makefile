NAME=taskmasterd

BIN_DIR = ./bin
CMD_DIR = ./cmd

all: $(NAME)

$(NAME):
	@go build -o $(BIN_DIR)/$@ $(CMD_DIR)/${NAME}/main.go 

clean:
	@rm -rf $(BIN_DIR)

.PHONY: all clean $(NAME)
