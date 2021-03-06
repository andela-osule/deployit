package service

import (
	"errors"
	"github.com/deployithq/deployit/daemon/env"
	"github.com/deployithq/deployit/drivers/interfaces"
	"github.com/satori/go.uuid"
	"strings"
)

type Service struct {
	UUID       string                `json:"uuid" yaml:"uuid"`
	Name       string                `json:"name" yaml:"name"`
	Tag        string                `json:"tag" yaml:"tag"`
	Containers map[string]*Container `json:"container" yaml:"container"`
	Config     Config                `json:"config" yaml:"config"`
}

type Container struct {
	ID string `json:"id" yaml:"id"`
}

func (s *Service) Get(e *env.Env, key string) error {
	e.Log.Info(`Get service `, key)

	if err := e.LDB.Read(key, s); err != nil {
		return err
	}

	return nil
}

func (s *Service) Update(e *env.Env) error {
	e.Log.Info(`Update service `, s.Name)

	if s.UUID == "" {
		return errors.New("service not found")
	}

	if err := e.LDB.Write(s.Name, s); err != nil {
		return err
	}

	return nil
}

func (s *Service) Create(e *env.Env, name string) error {
	e.Log.Info(`Create service `, name)

	u := uuid.NewV4()
	s.UUID = u.String()
	s.Name = name
	s.Tag = `latest`
	s.Containers = make(map[string]*Container)

	s.Config = Config{}
	s.Config.Get(e, name)

	if s.Config.Image == `` {
		return errors.New(`service not found`)
	}

	if err := e.LDB.Write(s.Name, s); err != nil {
		return err
	}

	return nil
}

func (s *Service) Pull(e *env.Env) error {
	e.Log.Info(`Pull service `, s.Config.Image)

	opts := interfaces.Image{
		Name: s.Config.Image,
		Auth: interfaces.AuthConfig{},
	}

	if err := e.Containers.PullImage(opts); err != nil {
		e.Log.Error(err)
		return err
	}

	if err := s.Update(e); err != nil {
		return err
	}

	return nil
}

func (s *Service) Start(e *env.Env) error {
	e.Log.Info(`Start service `, s.Name)

	if s.UUID == "" {
		return errors.New("service not found")
	}

	//TODO: implement scale

	hcfg := interfaces.HostConfig{
		Memory:     s.Config.Memory,
		Ports:      s.Config.Ports,
		Binds:      s.Config.Volumes,
		Privileged: false,
		RestartPolicy: interfaces.RestartPolicyConfig{
			Attempt: 10,
			Name:    "always",
		},
	}

	// Run containers if exists
	for _, container := range s.Containers {

		if err := e.Containers.StartContainer(&interfaces.Container{
			CID:        container.ID,
			HostConfig: hcfg,
		}); err != nil {
			e.Log.Error(err)
			return err
		}
	}

	for len(s.Containers) == 0 {

		c := &interfaces.Container{
			Config: interfaces.Config{
				Image:   s.Config.Image,
				Memory:  s.Config.Memory,
				Ports:   s.Config.Ports,
				Volumes: s.Config.Volumes,
				Env:     s.Config.Env,
			},
			HostConfig: hcfg,
		}

		if err := e.Containers.StartContainer(c); err != nil {
			e.Log.Error(err)
			return err
		}

		s.Containers[c.CID] = &Container{
			ID: c.CID,
		}
	}

	if err := s.Update(e); err != nil {
		return err
	}

	return nil
}

func (s *Service) Stop(e *env.Env) error {
	e.Log.Info(`Stop service `, s.Name)

	if s.UUID == "" {
		return errors.New("service not found")
	}

	for _, container := range s.Containers {

		if container.ID == "" {
			continue
		}

		if err := e.Containers.StopContainer(&interfaces.Container{
			CID: container.ID,
		}); err != nil {
			e.Log.Error(err)
			return err
		}
	}

	if err := s.Update(e); err != nil {
		return err
	}

	return nil
}

func (s *Service) Restart(e *env.Env) error {
	e.Log.Info(`Restart service `, s.Name)

	//TODO: implement start with configs
	//TODO: implement scale

	if err := s.Update(e); err != nil {
		return err
	}

	hcfg := interfaces.HostConfig{
		Memory:     s.Config.Memory,
		Ports:      s.Config.Ports,
		Binds:      s.Config.Volumes,
		Privileged: false,
		RestartPolicy: interfaces.RestartPolicyConfig{
			Attempt: 10,
			Name:    "always",
		},
	}

	// Run containers if exists
	for _, container := range s.Containers {
		if err := e.Containers.RestartContainer(&interfaces.Container{
			CID:        container.ID,
			HostConfig: hcfg,
		}); err != nil {
			e.Log.Error(err)
			return err
		}
	}

	for len(s.Containers) == 0 {
		c := &interfaces.Container{
			Config: interfaces.Config{
				Image:   s.Config.Image,
				Memory:  s.Config.Memory,
				Ports:   s.Config.Ports,
				Volumes: s.Config.Volumes,
				Env:     s.Config.Env,
			},
			HostConfig: hcfg,
		}

		if err := e.Containers.StartContainer(c); err != nil {
			e.Log.Error(err)
			return err
		}

		s.Containers[c.CID] = &Container{
			ID: c.CID,
		}
	}

	return nil
}

func (s *Service) Remove(e *env.Env) error {
	e.Log.Info(`Remove service `, s.Name)

	if s.UUID == "" {
		return errors.New("service not found")
	}

	for key, container := range s.Containers {
		if container.ID != "" {
			if err := e.Containers.RemoveContainer(&interfaces.Container{
				CID: container.ID,
			}); err != nil {
				e.Log.Error(err)
				index := strings.Index(err.Error(), "No such container")
				if index != -1 {
					e.Log.Info(`Clear record in db `, s.Name)
					delete(s.Containers, key)
					continue
				}

				return err
			}
		}

		delete(s.Containers, key)
	}

	if err := s.Update(e); err != nil {
		return err
	}

	return nil
}

func (s *Service) Destroy(e *env.Env) error {
	e.Log.Info(`Destroy service `, s.Name)

	if s.UUID == "" {
		return errors.New("service not found")
	}

	if err := s.Remove(e); err != nil {
		return err
	}

	if err := e.LDB.Remove(s.UUID); err != nil {
		return err
	}

	return nil
}

// Todo: temporary solution
func (s *Service) Ports(e *env.Env) (int64, error) {

	var port int64

	if len(s.Containers) == 0 {
		return port, nil
	}

	for _, container := range s.Containers {
		ports, err := e.Containers.InspectContainers(&interfaces.Container{
			CID: container.ID,
		})

		if err != nil {
			e.Log.Error(err)
			return port, err
		}

		port = ports[0]

		break
	}

	return port, nil
}
