package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/BurntSushi/toml"
)

var (
	cfgpath = flag.String("config", "cfg.toml", "config path")
)

type sshServer struct {
	Addr     string
	User     string
	Password string
	KeyPath  string
}
type config struct {
	SSH   sshServer
	Ports map[string]string
}

type session struct {
	client     *ssh.Client
	listenAddr string
	remoteAddr string
}

func newSession(listen, remote string, client *ssh.Client) *session {
	return &session{
		client:     client,
		listenAddr: listen,
		remoteAddr: remote,
	}
}

func (s *session) handleConn(conn net.Conn) {
	log.Printf("accept %s", conn.RemoteAddr())
	remote, err := s.client.Dial("tcp", s.remoteAddr)
	if err != nil {
		log.Printf("dial %s error", s.remoteAddr)
		return
	}
	log.Printf("%s -> %s connected.", conn.RemoteAddr(), s.remoteAddr)
	wait := new(sync.WaitGroup)
	wait.Add(2)
	go func() {
		io.Copy(remote, conn)
		remote.Close()
		wait.Done()
	}()
	go func() {
		io.Copy(conn, remote)
		conn.Close()
		wait.Done()
	}()
	wait.Wait()
	log.Printf("%s -> %s closed", conn.RemoteAddr(), s.remoteAddr)
}

func (s *session) Run() error {
	l, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go s.handleConn(conn)
	}
}

func login(cfg *sshServer) (*ssh.Client, error) {
	var methods []ssh.AuthMethod
	if cfg.KeyPath == "" && cfg.Password == "" {
		return nil, fmt.Errorf("empty private key and password")
	}

	if cfg.KeyPath != "" {
		key, err := ioutil.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read private key: %v", err)
		}

		// Create the Signer for this private key.
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			log.Fatalf("unable to parse private key: %v", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}

	sshconfig := &ssh.ClientConfig{
		User: cfg.User,
		Auth: methods,
	}

	return ssh.Dial("tcp", cfg.Addr, sshconfig)
}

func main() {
	flag.Parse()

	var cfg config
	_, err := toml.DecodeFile(*cfgpath, &cfg)
	if err != nil {
		log.Fatal(err)
	}

	client, err := login(&cfg.SSH)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("dial %s success", cfg.SSH.Addr)

	log.Printf("ports: %v", cfg.Ports)
	for local, remote := range cfg.Ports {
		sess := newSession(":"+local, remote, client)
		go func(local, remote string) {
			log.Printf("run session %s -> %s", local, remote)
			err := sess.Run()
			if err != nil {
				log.Fatalf("run %s error:%s", local, err)
			}
		}(local, remote)
	}

	select {}

}
