// telegram bot for running opencv python scripts
// on raspberry pi with camera module
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	bot "github.com/meinside/telegram-bot-go"
)

type Status int16

const (
	StatusWaiting Status = iota
)

const (
	DefaultMonitorIntervalSeconds = 5 // for monitoring

	NumQueue = 4 // size of queue

	// commands
	CommandStart    = "/start"
	CommandExecute  = "/execute"
	CommandShowCode = "/showcode"

	// messages
	MessageDefault        = "Input your command:"
	MessageUnknownCommand = "Unknown command."
	MessageErrorFormat    = "Error: %s"
)

type Session struct {
	UserId        string
	CurrentStatus Status
}

// session pool for storing individual statuses
type SessionPool struct {
	Sessions map[string]Session
	sync.Mutex
}

// for making sure the camera is not used simultaneously
var executeLock sync.Mutex

type ExecuteRequest struct {
	ChatId         interface{}
	MessageOptions map[string]interface{}
}

// variables
var apiToken string
var monitorInterval int
var isVerbose bool
var allowedIds []string
var scriptPath string
var pool SessionPool
var executeChannel chan ExecuteRequest

// keyboards
var allKeyboards = [][]bot.KeyboardButton{
	bot.NewKeyboardButtons(CommandExecute),
	bot.NewKeyboardButtons(CommandShowCode),
}

const (
	// constants for config
	ConfigFilename = "config.json"
)

// struct for config file
type Config struct {
	ApiToken        string   `json:"api_token"`
	AllowedIds      []string `json:"allowed_ids"`
	MonitorInterval int      `json:"monitor_interval"`
	ScriptPath      string   `json:"script_path"`
	IsVerbose       bool     `json:"is_verbose"`
}

// Read config
func getConfig() (config Config, err error) {
	_, filename, _, _ := runtime.Caller(0) // = __FILE__

	if file, err := ioutil.ReadFile(filepath.Join(path.Dir(filename), ConfigFilename)); err == nil {
		var conf Config
		if err := json.Unmarshal(file, &conf); err == nil {
			return conf, nil
		} else {
			return Config{}, err
		}
	} else {
		return Config{}, err
	}
}

// read code from the python script
func readCode() string {
	if bytes, err := ioutil.ReadFile(scriptPath); err == nil {
		return string(bytes)
	} else {
		return fmt.Sprintf(MessageErrorFormat, err)
	}
}

// initialization
func init() {
	// read variables from config file
	if config, err := getConfig(); err == nil {
		apiToken = config.ApiToken
		allowedIds = config.AllowedIds
		monitorInterval = config.MonitorInterval
		if monitorInterval <= 0 {
			monitorInterval = DefaultMonitorIntervalSeconds
		}
		scriptPath = config.ScriptPath
		isVerbose = config.IsVerbose

		// initialize session variables
		sessions := make(map[string]Session)
		for _, v := range allowedIds {
			sessions[v] = Session{
				UserId:        v,
				CurrentStatus: StatusWaiting,
			}
		}
		pool = SessionPool{
			Sessions: sessions,
		}

		// channels
		executeChannel = make(chan ExecuteRequest, NumQueue)
	} else {
		panic(err.Error())
	}
}

// check if given Telegram id is available
func isAvailableId(id string) bool {
	for _, v := range allowedIds {
		if v == id {
			return true
		}
	}
	return false
}

// process incoming update from Telegram
func processUpdate(b *bot.Bot, update bot.Update) bool {
	// check username
	var userId string
	if update.Message.From.Username == nil {
		log.Printf("*** Not allowed (no user name): %s", update.Message.From.FirstName)
		return false
	}
	userId = *update.Message.From.Username
	if !isAvailableId(userId) {
		log.Printf("*** Id not allowed: %s", userId)
		return false
	}

	// process result
	result := false

	pool.Lock()
	if session, exists := pool.Sessions[userId]; exists {
		// text from message
		var txt string
		if update.Message.HasText() {
			txt = *update.Message.Text
		} else {
			txt = ""
		}

		var message string
		var options map[string]interface{} = map[string]interface{}{
			"reply_markup": bot.ReplyKeyboardMarkup{
				Keyboard:       allKeyboards,
				ResizeKeyboard: true,
			},
			//"parse_mode": bot.ParseModeMarkdown,
		}

		switch session.CurrentStatus {
		case StatusWaiting:
			switch {
			// start
			case strings.HasPrefix(txt, CommandStart):
				message = MessageDefault
			// execute
			case strings.HasPrefix(txt, CommandExecute):
				message = ""
			// show code
			case strings.HasPrefix(txt, CommandShowCode):
				message = readCode()
			// fallback
			default:
				message = fmt.Sprintf("%s: %s", txt, MessageUnknownCommand)
			}
		}

		if len(message) > 0 {
			// 'typing...'
			b.SendChatAction(update.Message.Chat.Id, bot.ChatActionTyping)

			// send message
			if sent := b.SendMessage(update.Message.Chat.Id, message, options); sent.Ok {
				result = true
			} else {
				log.Printf("*** Failed to send message: %s", *sent.Description)
			}
		} else {
			// push to execute request channel
			executeChannel <- ExecuteRequest{
				ChatId:         update.Message.Chat.Id,
				MessageOptions: options,
			}
		}
	} else {
		log.Printf("*** Session does not exist for id: %s", userId)
	}
	pool.Unlock()

	return result
}

// process execute request
func processExecuteRequest(b *bot.Bot, request ExecuteRequest) bool {
	// process result
	result := false

	executeLock.Lock()
	defer executeLock.Unlock()

	// 'typing...'
	b.SendChatAction(request.ChatId, bot.ChatActionTyping)

	// execute script, read its output, and send it to the client
	if bytes, err := exec.Command(scriptPath).CombinedOutput(); err != nil {
		message := fmt.Sprintf("Error running script: %s (%s)", err, string(bytes))
		log.Printf("*** %s", message)

		if sent := b.SendMessage(request.ChatId, message, request.MessageOptions); sent.Ok {
			result = true
		} else {
			log.Printf("*** Failed to send error message: %s", *sent.Description)
		}
	} else {
		mime := http.DetectContentType(bytes)

		if strings.HasPrefix(mime, "image") { // image type
			b.SendChatAction(request.ChatId, bot.ChatActionUploadPhoto)

			if sent := b.SendPhoto(request.ChatId, bot.InputFileFromBytes(bytes), request.MessageOptions); sent.Ok {
				result = true
			} else {
				message := fmt.Sprintf("Failed to send photo: %s", *sent.Description)
				log.Printf("*** %s", message)

				if sent := b.SendMessage(request.ChatId, message, request.MessageOptions); sent.Ok {
					result = true
				} else {
					log.Printf("*** Failed to send error message: %s", *sent.Description)
				}
			}
		} else if strings.HasPrefix(mime, "video") { // video type
			b.SendChatAction(request.ChatId, bot.ChatActionUploadVideo)

			if sent := b.SendVideo(request.ChatId, bot.InputFileFromBytes(bytes), request.MessageOptions); sent.Ok {
				result = true
			} else {
				message := fmt.Sprintf("Failed to send video: %s", *sent.Description)
				log.Printf("*** %s", message)

				if sent := b.SendMessage(request.ChatId, message, request.MessageOptions); sent.Ok {
					result = true
				} else {
					log.Printf("*** Failed to send error message: %s", *sent.Description)
				}
			}
		} else {
			message := string(bytes)

			if sent := b.SendMessage(request.ChatId, message, request.MessageOptions); sent.Ok {
				result = true
			} else {
				log.Printf("*** Failed to send message: %s", *sent.Description)
			}
		}
	}

	return result
}

func main() {
	client := bot.NewClient(apiToken)
	client.Verbose = isVerbose

	// get info about this bot
	if me := client.GetMe(); me.Ok {
		log.Printf("Launching bot: @%s (%s)", *me.Result.Username, me.Result.FirstName)

		// delete webhook (getting updates will not work when wehbook is set up)
		if unhooked := client.DeleteWebhook(); unhooked.Ok {
			// monitor execution request channel
			go func() {
				for {
					select {
					case request := <-executeChannel:
						processExecuteRequest(client, request) // request execution of the script
					}
				}
			}()

			// wait for new updates
			client.StartMonitoringUpdates(0, monitorInterval, func(b *bot.Bot, update bot.Update, err error) {
				if err == nil {
					if update.Message != nil {
						processUpdate(b, update)
					}
				} else {
					log.Printf("*** Error while receiving update (%s)", err.Error())
				}
			})
		} else {
			panic("Failed to delete webhook")
		}
	} else {
		panic("Failed to get info of the bot")
	}
}
