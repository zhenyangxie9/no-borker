package gol

import (
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	ioKeypress <-chan rune
}

func NewWorld(height int, width int) [][]uint8 {
	newWorld := make([][]uint8, height)
	for i := range newWorld {
		newWorld[i] = make([]uint8, width)
	}
	return newWorld
}

func World(c distributorChannels, p Params) [][]uint8 {
	width := p.ImageWidth
	height := p.ImageHeight
	world := NewWorld(p.ImageHeight, p.ImageWidth)
	for h := 0; h < height; h++ {
		for w := 0; w < width; w++ {
			world[h][w] = <-c.ioInput
		}
	}
	return world
}

func output(p Params, c distributorChannels, world [][]uint8, turn int) {
	c.ioCommand <- ioOutput
	filename2 := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)
	c.ioFilename <- filename2
	for h := 0; h < p.ImageHeight; h++ {
		for w := 0; w < p.ImageWidth; w++ {
			c.ioOutput <- world[h][w]
		}
	}
	c.events <- ImageOutputComplete{turn, filename2}
}

func calculateAliveCells(p Params, world [][]uint8) []util.Cell {
	aliveCell := make([]util.Cell, 0)
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			if world[x][y] == 255 {
				aliveCell = append(aliveCell, util.Cell{X: y, Y: x})
			}
		}
	}
	return aliveCell
}

//func makeCallWorld(client *rpc.Client, world [][]uint8, p Params) [][]uint8 {
//	request := stubs.Request{World: world, Turns: p.Turns, Threads: p.Threads, ImageHeight: p.ImageHeight, ImageWidth: p.ImageWidth}
//	response := new(stubs.Response)
//	client.Call(stubs.Gameoflife, request, response)
//	return response.World
//}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Create a 2D slice to store the world.
	//server := "127.0.0.1:8040"
	//flag.Parse()
	client, _ := rpc.Dial("tcp", "3.87.225.255:8030")
	defer client.Close()
	c.ioCommand <- ioInput
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioFilename <- filename
	world := World(c, p)
	//server := flag.String("server", "127.0.0.1:8040", "IP:port string to connect to as server")

	for h := 0; h < p.ImageHeight; h++ {
		for w := 0; w < p.ImageWidth; w++ {
			if world[h][w] == 255 {
				c.events <- CellFlipped{0, util.Cell{h, w}}
			}
		}
	}
	turn := 0
	ticker := time.NewTicker(2 * time.Second)
	//pauseTicker := make(chan bool)
	go func() {
		for {
			select {
			//case TF := <-pauseTicker:
			//	if TF == true {
			//		<-pauseTicker
			//	}
			case <-ticker.C:
				req := new(stubs.Request)
				res := new(stubs.Response)
				err := client.Call(stubs.AliveCell, req, res)
				if err != nil {
					fmt.Println(err)
				}
				c.events <- AliveCellsCount{res.Turns, len(calculateAliveCells(p, res.World))}
				c.events <- TurnComplete{res.Turns}

			default:
				time.Sleep(time.Second)
			}
		}
	}()

	go func() {
		//pause := false
		for {
			req := new(stubs.Request)
			res := new(stubs.Response)
			keys := <-c.ioKeypress
			switch keys {
			case 's':
				client.Call(stubs.CurrentState, req, res)
				output(p, c, res.World, res.Turns)

			case 'q':

				client.Call(stubs.CurrentState, req, res)
				client.Close()
				close(c.events)
				os.Exit(0)
			case 'k':
				client.Call(stubs.CurrentState, req, res)
				output(p, c, res.World, res.Turns)
				client.Call(stubs.ShutDown, req, res)
				os.Exit(0)

			case 'p':
				client.Call(stubs.Pause, req, res)
				fmt.Println("turn is", res.Turns)
				for {
					if <-c.ioKeypress == 'p' {
						fmt.Println("Continuing")
						client.Call(stubs.Reset, req, res)
						break
					}
					//pauseTicker <- pause

				}
			}
		}

	}()
	//		//		//newWorld := makeCall(client, world, p)
	//		//		//for y := 0; y < p.ImageHeight; y++ {
	//		//		//	for x := 0; x < p.ImageWidth; x++ {
	//		//		//		if world[y][x] != newWorld[y][x] {
	//		//		//			c.events <- CellFlipped{turn, util.Cell{y, x}}
	//		//		//		}
	//		//		//	}
	//		//		//}
	//		//		world = makeCall(client, world, p)

	// TODO: Execute all turns of the Game of Life.

	// TODO: Report the final state using FinalTurnCompleteEvent.

	req := stubs.Request{World: world, Turns: p.Turns, Threads: p.Threads, ImageHeight: p.ImageHeight, ImageWidth: p.ImageWidth}
	res := new(stubs.Response)
	client.Call(stubs.Gameoflife, req, &res)
	turn = res.Turns
	world = res.World
	//c.events <- TurnComplete{turn}
	//c.events <- FinalTurnComplete{turn, makeCallAlive(client, world, p)}
	ticker.Stop()
	c.events <- FinalTurnComplete{turn, calculateAliveCells(p, world)}
	//filename2 := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)
	output(p, c, world, turn)
	//c.events <- ImageOutputComplete{turn, filename2}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{
		turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
