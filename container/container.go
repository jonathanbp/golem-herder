package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"

	docker "github.com/fsouza/go-dockerclient"
	log "github.com/sirupsen/logrus"
)

// GetAvailableHostPort returns an available (and random) port on the host machine
func GetAvailableHostPort() int {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// List containers matching the given predicate.
func List(client *docker.Client, matches func(container *docker.APIContainers) bool, all bool) ([]docker.APIContainers, error) {

	// Create client if it is not given
	if client == nil {
		c, err := docker.NewClientFromEnv()
		if err != nil {
			log.WithError(err).Error("Could not create docker client")
			return nil, err
		}
		client = c
	}

	containers, err := client.ListContainers(docker.ListContainersOptions{All: all})
	if err != nil {
		log.WithError(err).Error("Error listing containers")
		return nil, err
	}

	matching := []docker.APIContainers{}
	for _, container := range containers {
		if matches(&container) {
			matching = append(matching, container)
		}
	}
	return matching, nil
}

// And returns a container matching function composed of and'ing its argument funcs
func And(a func(container *docker.APIContainers) bool, b func(container *docker.APIContainers) bool) func(container *docker.APIContainers) bool {
	return func(container *docker.APIContainers) bool {
		return a(container) && b(container)
	}
}

// Or returns a container matching function composed of or'ing its argument funcs
func Or(a func(container *docker.APIContainers) bool, b func(container *docker.APIContainers) bool) func(container *docker.APIContainers) bool {
	return func(container *docker.APIContainers) bool {
		return a(container) || b(container)
	}
}

// WithID returns a func to match a containers ID (for use with e.g. List)
func WithID(id string) func(container *docker.APIContainers) bool {
	return func(container *docker.APIContainers) bool {
		return container.ID == id
	}
}

// WithName returns a func to match a containers name (for use with e.g. List)
func WithName(name string) func(container *docker.APIContainers) bool {
	return func(container *docker.APIContainers) bool {
		for _, containerName := range container.Names {
			if containerName == "/"+name {
				return true
			}
		}
		return false
	}
}

// WithLabel returns a func to match containers based on their label (for use with e.g. List)
func WithLabel(label, value string) func(container *docker.APIContainers) bool {
	return func(container *docker.APIContainers) bool {
		if labelValue, ok := container.Labels[label]; ok && value == labelValue {
			return true
		}
		return false
	}
}

// WithState returns a func to match a container's state (for use with e.g. List)
func WithState(state string) func(container *docker.APIContainers) bool {
	return func(container *docker.APIContainers) bool {
		fmt.Println(container.State)
		return container.State == state
	}
}

// LoadFiles will load the given files into the dir for container usage
func LoadFiles(dir string, files map[string][]byte) error {

	var wg sync.WaitGroup

	// write stuff to tmp dir
	for name, content := range files {
		// check if content is something we need to fetch
		if url, err := url.Parse(string(content)); err == nil && strings.HasPrefix(string(content), "http") {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				request, err := http.NewRequest("GET", url.String(), nil)
				if err != nil {
					log.WithError(err).WithField("url", url.String()).Warn("Could not create request")
				}
				// we need to add basic auth for webstrates assets
				if url.Hostname() == "webstrates.cs.au.dk" || url.Hostname() == "hiraku.cs.au.dk" {
					request.SetBasicAuth("web", "strate")
				}
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					log.WithError(err).WithField("file", name).WithField("url", url.String()).Warn("Could not GET content to store in container")
				}
				defer response.Body.Close()
				fetchedContent, err := ioutil.ReadAll(response.Body)
				if err != nil {
					log.WithError(err).WithField("url", url.String()).Warn("Error getting body")
				}
				// write content of url to file
				log.WithField("file", name).Info("Writing fetched content to tmp dir")
				err = ioutil.WriteFile(filepath.Join(dir, name), fetchedContent, 0644)
				if err != nil {
					log.WithError(err).WithField("file", name).Warn("Could not write file to tmp dir")
				}
			}(name)
		} else {
			// default case, something not an url
			log.WithField("file", name).Info("Writing provided content to tmp dir")
			err := ioutil.WriteFile(filepath.Join(dir, name), content, 0644)
			if err != nil {
				log.WithError(err).WithField("file", name).Warn("Could not write file to tmp dir")
				return err
			}
		}
	}

	// Wait for all async tasks to complete
	wg.Wait()
	return nil
}

// Kill the container with the given name and optionally remove mounted volumes.
func Kill(matcher func(container *docker.APIContainers) bool, removeContainer, destroyData bool) error {

	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.WithError(err).Error("Could not create docker client")
		return err
	}

	containers, err := List(client, matcher, false)
	if err != nil {
		log.WithError(err).Warn("Error listing containers")
		return err
	}
	if len(containers) != 1 {
		log.WithField("count", len(containers)).Warn("Too many or too few matching containers")
		return fmt.Errorf("Expected 1 container to match name, got %v", len(containers))
	}

	log.WithField("container", containers[0].ID).Info("Killing container")
	err = client.KillContainer(docker.KillContainerOptions{ID: containers[0].ID})
	if err != nil {
		return err
	}

	if removeContainer {

		log.WithField("container", containers[0].ID).Info("Removing container")
		err = client.RemoveContainer(docker.RemoveContainerOptions{
			ID:            containers[0].ID,
			Force:         true,
			RemoveVolumes: destroyData,
		})
		if err != nil {
			log.WithError(err).Warn("Error removing container")
			return err
		}
	}

	return nil
}

func run(client *docker.Client, name, repository, tag string, ports map[int]int, mounts map[string]string, labels map[string]string, restart bool) (*docker.Container, error) {

	log.WithFields(log.Fields{"image": fmt.Sprintf("%s:%s", repository, tag)}).Info("Pulling image")

	err := client.PullImage(docker.PullImageOptions{
		Repository: repository,
		Tag:        tag,
	}, docker.AuthConfiguration{})
	if err != nil {
		return nil, err
	}

	// Construct []Mount
	ms := []docker.Mount{}
	binds := []string{}
	if mounts != nil {
		for s, d := range mounts {
			log.WithField("source", s).WithField("dest", d).Info("Preparing mount")
			ms = append(ms, docker.Mount{Source: s, Destination: d})
			binds = append(binds, fmt.Sprintf("%s:%s", s, d))
		}
	}

	// Construct port bindings
	exposedPorts := map[docker.Port]struct{}{}
	portBindings := map[docker.Port][]docker.PortBinding{}
	if ports != nil {
		for outsidePort, insidePort := range ports {
			insidePortTCP := docker.Port(fmt.Sprintf("%d/tcp", insidePort))
			exposedPorts[insidePortTCP] = struct{}{}
			portBindings[insidePortTCP] = []docker.PortBinding{{
				HostIP:   "0.0.0.0",
				HostPort: fmt.Sprintf("%d", outsidePort),
			},
			}
		}
	}

	container, err := client.CreateContainer(
		docker.CreateContainerOptions{
			Name: name,
			Config: &docker.Config{
				Image:        fmt.Sprintf("%s:%s", repository, tag),
				Labels:       labels,
				ExposedPorts: exposedPorts,
				Mounts:       ms,
				AttachStdout: true,
				AttachStderr: true,
				AttachStdin:  true,
				OpenStdin:    true,
				Tty:          true,
			},
			HostConfig: &docker.HostConfig{
				PortBindings: portBindings,
				Binds:        binds,
			},
		},
	)
	var containerID string
	if err != nil {
		log.WithError(err).WithField("restart", restart).Error("Error creating container")
		if !restart {
			return nil, err
		}
		// try finding container by name
		log.WithField("name", name).Info("Looking for container")
		containers, err := List(client, WithName(name), true)
		if err != nil {
			return nil, err
		}
		if len(containers) != 1 {
			return nil, fmt.Errorf("Could not create nor find container with name %s", name)
		}
		containerID = containers[0].ID
	} else {
		containerID = container.ID
	}

	log.WithField("containerID", containerID).Info("Created/found container")

	// Start container
	err = client.StartContainer(containerID, nil)
	if err != nil {
		log.WithError(err).Error("Error starting container")
		return nil, err
	}

	log.WithField("containerid", containerID).Info("Container (re-)started")

	c, err := client.InspectContainer(containerID)
	if err != nil {
		return container, nil
	}

	return c, nil
}

// RunDaemonized will pull, create and start the container piping stdout and stderr to the given channels.
// This function is meant to run longlived, persistent processes.
// A directory (/<name>) will be mounted in the container in which data which must be persisted between sessions can be kept.
func RunDaemonized(name, repository, tag string, ports map[int]int, files map[string][]byte, labels map[string]string, restart bool, stdout, stderr chan<- []byte, done chan<- bool) (*docker.Container, error) {

	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.WithError(err).Error("Could not create docker client")
		return nil, err
	}

	hostdir := path.Join(viper.GetString("mounts"), name)

	// Construct dir if not exists
	if err := os.MkdirAll(hostdir, 0777); err != nil {
		return nil, err
	}

	// Construct mounts
	mounts := map[string]string{
		hostdir: fmt.Sprintf("/%v", name),
		hostdir: "/minion", // also put file in minion directory for minion compat
	}

	if err := LoadFiles(hostdir, files); err != nil {
		return nil, err
	}

	c, err := run(client, name, repository, tag, ports, mounts, labels, restart)
	if err != nil {
		return nil, err
	}

	// Setup monitor for service - if it does done should be notified
	if done != nil {
		go func() {
			// Cleanup container after it exits
			defer func() {
				err = client.RemoveContainer(docker.RemoveContainerOptions{
					ID:            c.ID,
					Force:         true,
					RemoveVolumes: false,
				})
				if err != nil {
					log.WithError(err).Warn("Error removing container")
				}
			}()
			for {
				<-time.After(time.Second)
				if cntnr, err := client.InspectContainer(c.ID); err != nil || !cntnr.State.Running {
					log.WithField("name", name).WithField("id", c.ID).Info("Container looks dead")
					done <- true
					return
				}
			}
		}()
	}

	if stdout == nil || stderr == nil {
		return c, nil
	}

	// Use a pipe to run stdout and stderr to channels
	stdoutr, stdoutw := io.Pipe()
	stderrr, stderrw := io.Pipe()
	client.Logs(docker.LogsOptions{
		Stdout:       true,
		Container:    c.ID,
		OutputStream: stdoutw,
		ErrorStream:  stderrw,
	})

	// stdout goes to channel
	go func(r io.Reader, out chan<- []byte) {
		data := make([]byte, 512)
		_, err := r.Read(data)
		out <- data
		if err != nil {
			// stop looking for stdout
			return
		}
	}(stdoutr, stdout)

	// stderr goes to channel
	go func(r io.Reader, out chan<- []byte) {
		data := make([]byte, 512)
		_, err := r.Read(data)
		out <- data
		if err != nil {
			// stop looking for stderr
			return
		}
	}(stderrr, stderr)

	return c, nil
}

// RunLambda will pull, create and start the container returning its stdout.
// This function is meant to run a shortlived process.
func RunLambda(ctx context.Context, name, repository, tag string, mounts map[string]string) ([]byte, []byte, error) {

	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.WithError(err).Error("Could not create docker client")
		return nil, nil, err
	}

	container, err := run(client, name, repository, tag, nil, mounts, nil, false)
	if err != nil {
		return nil, nil, err
	}

	// Cleanup
	defer func() {
		err = client.RemoveContainer(docker.RemoveContainerOptions{
			ID:            container.ID,
			Force:         true,
			RemoveVolumes: true,
		})
		if err != nil {
			log.WithError(err).Warn("Error removing container")
		}
	}()

	_, err = client.WaitContainerWithContext(container.ID, ctx)
	if err != nil {
		log.WithError(err).Warn("Error waiting for container to exit")
		return nil, nil, err
	}

	// Use a buffer to capture output
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = client.Logs(docker.LogsOptions{
		Stdout:       true,
		Container:    container.ID,
		RawTerminal:  true,
		OutputStream: &stdout,
		ErrorStream:  &stderr,
	})
	if err != nil {
		log.WithError(err).Warn("Error getting container logs")
	}

	log.WithField("stdout", stdout.String()).WithField("stderr", stderr.String()).Info("Run done")

	return stdout.Bytes(), stderr.Bytes(), nil
}

// Attach to a container
func Attach(c docker.APIContainers, stdout, stderr chan<- []byte, stdin <-chan []byte) error {

	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.WithError(err).Error("Could not create docker client")
		return err
	}

	// Use a pipe to run stdout and stderr to channels
	stdoutr, stdoutw := io.Pipe()
	stderrr, stderrw := io.Pipe()
	stdinr, stdinw := io.Pipe()

	cw, err := client.AttachToContainerNonBlocking(docker.AttachToContainerOptions{
		Container:    c.ID,
		Logs:         true,
		Stream:       true,
		Stdout:       true,
		Stderr:       true,
		Stdin:        true,
		RawTerminal:  true,
		OutputStream: stdoutw,
		ErrorStream:  stderrw,
		InputStream:  stdinr})
	if err != nil {
		log.WithError(err).Warn("Could not attach")
		return err
	}

	// stdout goes to channel
	go func(r io.Reader, out chan<- []byte, c io.Closer) {
		for {
			data := make([]byte, 512)
			_, err := r.Read(data)
			out <- data
			if err != nil {
				// stop looking for stdout
				c.Close()
				return
			}
		}
	}(stdoutr, stdout, cw)

	// stderr goes to channel
	go func(r io.Reader, out chan<- []byte, c io.Closer) {
		for {
			data := make([]byte, 512)
			_, err := r.Read(data)
			out <- data
			if err != nil {
				// stop looking for stderr
				c.Close()
				return
			}
		}
	}(stderrr, stderr, cw)

	// stdin goes from channel
	go func(w io.Writer, in <-chan []byte, c io.Closer) {
		for line := range in {
			_, err := w.Write(line)
			if err != nil {
				// stop looking for stdin
				c.Close()
				return
			}
		}
	}(stdinw, stdin, cw)
	return nil
}
