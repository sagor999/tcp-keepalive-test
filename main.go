package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	VERSION string = "latest"
	COMMIT  string = "HEAD"
)

func init() {
	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

func main() {
	var cmd = &cobra.Command{
		Use:   "tcp-keepalive-test",
		Short: "test for tcp keepalive",
		RunE:  run,
	}

	cmd.Flags().String("mode", "client", "client or server mode")
	cmd.Flags().Int("listen", 9797, "listen port for server")
	cmd.Flags().String("connect", "localhost:9797", "connect string for client")
	cmd.Flags().Int("num-clients", 1, "number of clients to spawn")
	cmd.Flags().Int("wait", 60, "time in min to wait before sending reply")

	err := cmd.Execute()
	if err != nil {
		log.Errorf("Error: %s\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return err
	}

	listen, err := cmd.Flags().GetInt("listen")
	if err != nil {
		return err
	}

	connect, err := cmd.Flags().GetString("connect")
	if err != nil {
		return err
	}

	numClients, err := cmd.Flags().GetInt("num-clients")
	if err != nil {
		return err
	}

	wait, err := cmd.Flags().GetInt("wait")
	if err != nil {
		return err
	}

	if mode == "server" {
		return serve(listen, wait)
	} else {
		var wg sync.WaitGroup
		for i := 1; i <= numClients; i++ {
			log.Infof("Spawning client %d/%d", i, numClients)
			wg.Add(1)
			go client(&wg, connect)
			log.Infof("Sleeping one minute before spawning next client")
			time.Sleep(time.Minute * 1)
		}
		wg.Wait()
	}
	return nil
}

func serve(listenPort, wait int) error {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{Port: listenPort})
	if err != nil {
		return err
	}
	defer l.Close()

	log.Infof("Listening on port %d", listenPort)

	for {
		c, err := l.AcceptTCP()
		if err != nil {
			return err
		}
		go handleConnection(c, wait)
	}
}

func setKeepaliveParameters(conn *net.TCPConn) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		log.Error("on getting raw connection object for keepalive parameter setting", err.Error())
	}

	rawConn.Control(
		func(fdPtr uintptr) {
			// got socket file descriptor. Setting parameters.
			fd := int(fdPtr)
			//Number of probes.
			err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, 3)
			if err != nil {
				log.Error("on setting keepalive probe count", err.Error())
			}
			//Wait time after an unsuccessful probe.
			err = syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, 3)
			if err != nil {
				log.Error("on setting keepalive retry interval", err.Error())
			}
		})
}

func handleConnection(c *net.TCPConn, wait int) {
	c.SetKeepAlive(true)
	c.SetKeepAlivePeriod(time.Second * 30)
	setKeepaliveParameters(c)

	log.Infof("Serving %s", c.RemoteAddr().String())
	for {
		netData, err := bufio.NewReader(c).ReadString('\n')
		if err != nil {
			log.Errorf("Error during reading from socket: %s", err)
			return
		}

		rcvd := strings.TrimSpace(string(netData))
		log.Infof("Received: %s", rcvd)

		log.Infof("Scheduled to reply in %d minutes", wait)
		go func() {
			time.Sleep(time.Minute * time.Duration(wait))
			log.Infof("Sending scheduled reply now")
			send := "PONG\n"
			c.Write([]byte(send))
		}()
	}
	c.Close()
}

func client(wg *sync.WaitGroup, connect string) {
	defer wg.Done()
	log.Infof("Connecting to server at %s", connect)
	conn, err := net.Dial("tcp", connect)
	if err != nil {
		log.Error("on connecting to server: ", err.Error())
		return
	}
	for {
		log.Info("Sending PING to server")
		fmt.Fprintf(conn, "PING\n")

		reply, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			log.Error("on reading from socket: ", err.Error())
			return
		}
		log.Infof("Received from server: %s", reply)
	}
}
