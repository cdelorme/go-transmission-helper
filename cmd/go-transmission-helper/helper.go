package main

import (
	"encoding/base64"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/cdelorme/go-log"
	"github.com/cdelorme/go-transmission-api"
	"github.com/cdelorme/gonf"
)

var errBadMovePath = errors.New("file exists at supplied path...")
var readfile = ioutil.ReadFile
var stat = os.Stat
var remove = os.Remove
var readdir = ioutil.ReadDir

type helper struct {
	transmission.Transmission
	log.Logger
	Add    string `json:"add,omitempty"`
	Remove bool   `json:"remove,omitempty"`
	Move   string `json:"move,omitempty"`
	File   string `json:"configFile,omitempty"`
}

func (self *helper) load64(f string) (string, error) {
	d, err := readfile(f)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(d), nil
}

func (self *helper) addFile(f string) error {
	self.Debug("adding torrent file %s", f)

	meta, err := self.load64(f)
	if err != nil {
		return err
	}

	if err := self.Transmission.Add(meta); err != nil {
		self.Error("transmission failed to add metadata: %s", err)
		return err
	}

	return nil
}

func (self *helper) add() error {
	if self.Add == "" {
		return nil
	}

	var d os.FileInfo
	var err error
	d, err = stat(self.Add)
	if err != nil {
		self.Warning("unable to read supplied path (%s): %s", self.Add, err)
		self.Add = path.Join(os.Getenv("HOME"), "Downloads")
		d, err = stat(self.Add)
		if err != nil {
			self.Error("unable to read downloads folder (%s)...", err)
			return err
		}
	}

	if !d.IsDir() && d.Mode().IsRegular() {
		if err = self.addFile(self.Add); err != nil {
			self.Error("failed to add %s (%s)", self.Add, err)
			return err
		}
		remove(self.Add)
	} else {
		files, err := readdir(self.Add)
		if err != nil {
			self.Error("failed to read files in %s...", self.Add)
			return err
		}

		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".torrent") {
				a := path.Join(self.Add, f.Name())
				if err = self.addFile(a); err != nil {
					self.Error("unable to load %s (%s)", a, err)
				} else {
					remove(a)
				}
			}
		}
	}

	return self.Transmission.Resume()
}

func (self *helper) move() error {
	if self.Move == "" {
		return nil
	}

	if fi, err := stat(self.Move); err == nil && !fi.IsDir() {
		self.Error("%s", errBadMovePath)
		return errBadMovePath
	}

	self.Debug("searching for finished torrents...")
	list, err := self.Transmission.Finished()
	if err != nil {
		self.Error("failed to get a list of completed torrents: %s", err)
		return err
	}
	self.Debug("list: %+v\n", list)

	self.Debug("moving finished torrent downloads to %s", self.Move)
	err = self.Transmission.Move(self.Move, list)
	if err != nil {
		self.Error("failed to move completed torrents: %s", err)
		return err
	}
	self.Debug("finished moving list...")

	if self.Remove {
		self.Debug("removing complete torrents from transmission...")
		err = self.Transmission.Remove(list)
		if err != nil {
			self.Error("failed to remove completed torrents: %s", err.Error())
			return err
		}
	}

	return nil
}

func (self *helper) Init() {
	g := gonf.Gonf{Description: "A utility to help wield the power of transmission through cli & automation", Configuration: self}
	g.Add("configFile", "transmission config file path", "TRANSMISSION_CONFIG", "-c:", "--config")
	g.Add("add", "add torrent(s) from the supplied path", "TRANSMISSION_ADD", "-a:", "--add")
	g.Add("move", "move torrents in finished state to this folder", "TRANSMISSION_MOVE", "-m:", "--move")
	g.Add("remove", "remove torrents in finished state from transmission", "TRANSMISSION_REMOVE", "-r", "--remove")
	g.Example("-a ~/Downloads")
	g.Example("-r -m /backup/drive/")
	g.Load()
}

func (self *helper) Run() int {
	if err := self.Transmission.Configure(self.File); err != nil {
		self.Error("Failed to read transmission configuration: %s", err.Error())
		return 1
	}

	var code int
	if self.add() != nil {
		code = 1
	}
	if self.move() != nil {
		code = 1
	}
	return code
}
