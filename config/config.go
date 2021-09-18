package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"github.com/BurntSushi/toml"
)

type ServerConfig struct {
	ServerPort  int    `toml:"serverPort"`
	LogFilePath string `toml:"logFilePath"`
}

type AuthConfig struct {
	MsgToken string `toml:"MsgToken"`
}

type Config struct {
	Server *ServerConfig	`toml:"server"`
	Auth   *AuthConfig		`toml:"auth"`
}

var configOnce = sync.Once{}
var config *Config

func InitConfig(configFile string){
	configOnce.Do(func() {
		config = &Config{
			Server: &ServerConfig{},
			Auth:   &AuthConfig{},
		}
		configString,err := ioutil.ReadFile(configFile)
		if err != nil{
			panic(err.Error())
		}

		_,err = toml.Decode(string(configString),&config)
		if err != nil{
			panic(err.Error())
		}

		//移除路径最后的分隔符，便于后续统一处理
		config.Server.LogFilePath = filepath.Dir(config.Server.LogFilePath)
		//检测日志路径是否存在
		err = os.Chdir(config.Server.LogFilePath)
		if err != nil{
			if os.IsNotExist(err){
				err = os.Mkdir(config.Server.LogFilePath,0666)
				if err != nil{
					panic(err.Error())
				}
			}else{
				panic(err.Error())
			}
		}
	})
}
//获取监听端口
func GetServerPort()int{
	if config == nil{
		panic("init config before get config field")
	}
	return config.Server.ServerPort
}
//获取指定日志文件
func GetLogFile()string{
	if config == nil{
		panic("init config before get config field")
	}
	return config.Server.LogFilePath
}
//获取鉴权token
func GetMsgToken()string{
	if config == nil{
		panic("init config before get config field")
	}
	return config.Auth.MsgToken
}



