package main

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// TODO: Timeouts for network stuff.
// FIXME: Should I look for tags case-insensitively? The documentation
//        doesn't say anything about this.

type mpdState int

const (
	playMPDState mpdState = iota
	stopMPDState
	pauseMPDState
)

func handleMpdEvents(events chan event) {
	for {
		changes, err := executeMPDCommand("idle")
		if err != nil {
			return
		}
		for _, line := range strings.Split(changes, "\n") {
			if strings.Contains(line, "playlist") ||
				strings.Contains(line, "player") {
				events <- updateStateEvent
				break
			}
		}
	}
}

func getState() (state state, err error) {
	status, err := executeMPDCommand("status")
	if err != nil {
		return
	}
	if state.mpdState, err = getMPDState(status); err != nil {
		return
	}
	if state.elapsed, err = getElapsed(status); err != nil {
		return
	}
	if state.songID, err = getSongID(status); err != nil {
		return
	}
	if err = fillQueue(&state); err != nil {
		return
	}
	return
}

func fillQueue(state *state) error {
	info, err := executeMPDCommand("playlistinfo")
	if err != nil {
		return err
	}
	var s song
	for _, line := range strings.Split(info, "\n") {
		split := strings.SplitN(line, ": ", 2)
		switch split[0] {
		case "file":
			if len(s.uri) > 0 {
				state.queue = append(state.queue, s)
				s = song{} // reset, before parsing the next song
			}
			if len(split) < 2 {
				return fmt.Errorf("encountered epmty URI")
			}
			s.uri = split[1]
		case "Id":
			if len(split) > 1 {
				if s.songID, err = strconv.Atoi(split[1]); err != nil {
					return fmt.Errorf("could not parse songid: %s", err.Error())
				}
			}
		case "duration":
			if len(split) > 1 {
				f, err := strconv.ParseFloat(split[1], 32)
				if err != nil {
					return fmt.Errorf("could not parse duration: %s", err.Error())
				}
				s.duration = float32(f)
			}
		case "Title":
			if len(split) > 1 {
				s.title = split[1]
			}
		case "Artist":
			if len(split) > 1 {
				s.artist = split[1]
			}
		case "Album":
			if len(split) > 1 {
				s.album = split[1]
			}
		case "Track":
			if len(split) > 1 {
				i, err := strconv.Atoi(split[1])
				if err != nil {
					return fmt.Errorf("could not parse track: %s", err.Error())
				}
				s.track = &i
			}
		}
	}
	if len(s.uri) > 0 {
		state.queue = append(state.queue, s) // add last song to queue
	}
	return nil
}

func getMPDState(status string) (mpdState mpdState, err error) {
	for _, line := range strings.Split(status, "\n") {
		if strings.HasPrefix(line, "state: ") && len(line) > 7 {
			switch line[7:] {
			case "play":
				return playMPDState, nil
			case "stop":
				return stopMPDState, nil
			case "pause":
				return pauseMPDState, nil
			}
		}
	}
	err = fmt.Errorf("mpdState not found")
	return
}

func getElapsed(status string) (*float32, error) {
	for _, line := range strings.Split(status, "\n") {
		if strings.HasPrefix(line, "elapsed: ") && len(line) > 9 {
			s, err := strconv.ParseFloat(line[9:], 32)
			s32 := float32(s)
			return &s32, err
		}
	}
	return nil, nil
}

func getSongID(status string) (*int, error) {
	for _, line := range strings.Split(status, "\n") {
		if strings.HasPrefix(line, "songid: ") && len(line) > 8 {
			i, err := strconv.Atoi(line[8:])
			return &i, err
		}
	}
	return nil, nil
}

func executeMPDCommand(command string) (resp string, err error) {
	conn, err := initiateMPDConnection()
	if conn != nil {
		defer conn.Close()
	}
	if err != nil {
		return
	}
	fmt.Fprintf(conn, "%s\n", command)
	var respBuilder strings.Builder
	var line string
	connReader := bufio.NewReader(conn)
	for {
		if line, err = connReader.ReadString('\n'); err != nil {
			return
		}
		if line == "OK\n" {
			break
		} else if strings.HasPrefix(line, "ACK ") {
			err = fmt.Errorf("received mpd error %s", line)
			break
		} else {
			respBuilder.WriteString(line)
		}
	}
	return respBuilder.String(), err
}

func initiateMPDConnection() (conn *net.TCPConn, err error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", mpdAddr)
	if err != nil {
		return
	}
	conn, err = net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return
	}
	var line string
	connReader := bufio.NewReader(conn)
	if line, err = connReader.ReadString('\n'); err != nil {
		return
	}
	if !strings.HasPrefix(line, "OK MPD ") {
		err = fmt.Errorf("no mpd server found")
	}
	return
}

func playHighlighted(state state) error {
	if len(state.queue) == 0 {
		return nil
	}
	song := state.queue[state.highlighted]
	_, err := executeMPDCommand(fmt.Sprintf("playid %d", song.songID))
	return err
}

func togglePause(state state) error {
	switch state.mpdState {
	case playMPDState:
		_, err := executeMPDCommand("pause 1")
		return err
	case pauseMPDState:
		_, err := executeMPDCommand("pause 0")
		return err
	}
	return nil
}

func deleteHighlighted(state state) error {
	if len(state.queue) == 0 {
		return nil
	}
	song := state.queue[state.highlighted]
	_, err := executeMPDCommand(fmt.Sprintf("deleteid %d", song.songID))
	return err
}

func moveHighlightedUpwards(state *state) error {
	if state.highlighted == 0 {
		// Can't move over the top. Just ignore.
		return nil
	}
	cmd := fmt.Sprintf("move %d %d\n", state.highlighted, state.highlighted-1)
	state.highlighted -= 1
	_, err := executeMPDCommand(cmd)
	return err
}

func moveHighlightedDownwards(state *state) error {
	if state.highlighted >= len(state.queue)-1 {
		// Can't move below the bottom. Just ignore.
		return nil
	}
	cmd := fmt.Sprintf("move %d %d\n", state.highlighted, state.highlighted+1)
	state.highlighted += 1
	_, err := executeMPDCommand(cmd)
	return err
}

func seekBackwards(seconds int) error {
	_, err := executeMPDCommand(fmt.Sprintf("seekcur -%d", seconds))
	return err
}

func seekForwards(seconds int) error {
	_, err := executeMPDCommand(fmt.Sprintf("seekcur +%d", seconds))
	if err != nil && strings.Contains(err.Error(), "Decoder failed to seek") {
		// This seems to happen, when trying to seek across the end of a song.
		return nil
	}
	return err
}