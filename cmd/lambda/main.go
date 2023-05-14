package main

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/joho/godotenv"
	"github.com/pkg/sftp"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

type Response struct {
	Message        string `json:"message"`
	Code           string `json:"code,omitempty"`
	NumMapsDeleted int    `json:"num_maps_deleted"`
}

const SUCCESS_MESSAGE = "Function finished without errors"
const CONNECTION_TIMEOUT_DURATION = 5 * time.Second

func makeAndLogErrorResponse(message string, code string, logger *zap.Logger) Response {
	response := Response{Message: message, Code: code}
	logger.Sugar().Error("Response: ", response)
	return response
}

func Handler(ctx context.Context) (Response, error) {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// set up the SFTP server details
	hostPort := os.Getenv("SFTP_HOST_PORT")
	username := os.Getenv("SFTP_USERNAME")
	password := os.Getenv("SFTP_PASSWORD")
	gameServerBasePath := os.Getenv("GAME_SERVER_BASE_PATH")
	if gameServerBasePath == "" {
		return makeAndLogErrorResponse("missing GAME_SERVER_BASE_PATH", "missing_evar", logger), errors.New("missing GAME_SERVER_BASE_PATH")
	}

	// set up the SSH client config
	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         CONNECTION_TIMEOUT_DURATION,
	}

	// connect to the SSH server with a timeout
	conn, err := ssh.Dial("tcp", hostPort, sshConfig)
	if err != nil {
		return makeAndLogErrorResponse("Error connecting to SFTP server", "sftp_connection_error", logger), err
	}
	defer conn.Close()

	// open an SFTP session on the SSH connection
	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return makeAndLogErrorResponse("Error connecting to SFTP server", "sftp_session_error", logger), err
	}
	defer sftpClient.Close()

	matches, err := sftpClient.Glob(gameServerBasePath + "/*.map")
	if err != nil {
		return makeAndLogErrorResponse("Failed to search for maps", "map_search_error", logger), err
	}

	numDeleted := 0
	for _, filePath := range matches {
		// Errors are intentionally ignored here as some game server hosts have misconfigured FTP servers that report errors even though everything went fine
		sftpClient.Rename(filePath, filePath+".softdeleted")
		numDeleted++
	}

	// return a success message
	logger.Info(SUCCESS_MESSAGE, zap.Int("num_maps_deleted", numDeleted))
	return Response{Message: SUCCESS_MESSAGE, NumMapsDeleted: numDeleted}, nil
}

func main() {
	godotenv.Load("../../.env")
	if os.Getenv("RUN_WITHOUT_LAMBDA") == "true" {
		Handler(context.TODO())
	} else {
		lambda.Start(Handler)
	}
}
