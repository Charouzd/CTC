package main

import (
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var werdun int
var timeRec = make(map[string]time.Duration)
var countRec = make(map[string]int)

type Config struct {
	Cars      CarConfig      `yaml:"cars"`
	Stations  StationConfig  `yaml:"stations"`
	Registers RegisterConfig `yaml:"registers"`
}

type CarConfig struct {
	Count          int    `yaml:"count"`
	ArrivalTimeMin string `yaml:"arrival_time_min"`
	ArrivalTimeMax string `yaml:"arrival_time_max"`
}

type StationConfig struct {
	Gas      []StationType `yaml:"gas"`
	Diesel   []StationType `yaml:"diesel"`
	LPG      []StationType `yaml:"lpg"`
	Electric []StationType `yaml:"electric"`
}

type StationType struct {
	Count        int    `yaml:"count"`
	ServeTimeMin string `yaml:"serve_time_min"`
	ServeTimeMax string `yaml:"serve_time_max"`
}

type RegisterConfig struct {
	Count         int    `yaml:"count"`
	HandleTimeMin string `yaml:"handle_time_min"`
	HandleTimeMax string `yaml:"handle_time_max"`
}

type Car struct {
	ID            int
	Type          int8
	arrivalTime   time.Time
	tankStartTime time.Time
	tankEndTime   time.Time
	payStart      time.Time
	leavingTime   time.Time
	station       *Station
}

type Station struct {
	ID           int
	Type         int8
	serveTimeMin time.Duration
	serveTimeMax time.Duration
	Queue        chan *Car // Change to pointer to Car
	status       bool
}

type Register struct {
	ID            int
	HandleTimeMin time.Duration
	HandleTimeMax time.Duration
	Queue         chan *Car // Change to pointer to Car
}

type InitQueue struct {
	Queue    chan Car
	MinDelay time.Duration
	MaxDelay time.Duration
}
type Stations struct {
	gas    []Station
	diesel []Station
	el     []Station
	lpg    []Station
}

func loadConfig(filename string) (Config, error) {
	var config Config
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	return config, err
}

func main() {
	timeRec["MinGasPass"] = time.Duration(10000000000)
	timeRec["MinDieselPass"] = time.Duration(1000000000)
	timeRec["MinLPGPass"] = time.Duration(1000000000)
	timeRec["MinElectricPass"] = time.Duration(1000000000)
	fmt.Println("Welcome to virtual gas station")
	// Load configuration
	config, err := loadConfig("conf.yaml")
	if err != nil {
		panic(err)
	}

	// Create stations based on configuration(yaml file)
	gasStations := createStations(config.Stations.Gas, 1)
	dieselStations := createStations(config.Stations.Diesel, 2)
	lpgStations := createStations(config.Stations.LPG, 3)
	electricStations := createStations(config.Stations.Electric, 4)
	stations := Stations{gas: gasStations, diesel: dieselStations, lpg: lpgStations, el: electricStations}

	// Create registers based on configuration(yaml file)
	registers := createRegisters(config.Registers)

	// Synch for group to process
	var wg sync.WaitGroup
	wg.Add(len(gasStations) + len(dieselStations) + len(lpgStations) + len(electricStations) + len(registers))
	confirmation := make(chan *Car)
	// Start processing cars arriving at each station
	for _, station := range gasStations {
		go processStation(station, registers, confirmation, &wg, stations, registers)
	}
	for _, station := range dieselStations {
		go processStation(station, registers, confirmation, &wg, stations, registers)
	}
	for _, station := range lpgStations {
		go processStation(station, registers, confirmation, &wg, stations, registers)
	}
	for _, station := range electricStations {
		go processStation(station, registers, confirmation, &wg, stations, registers)
	}

	// Start processing cars at each register
	for _, register := range registers {
		go processRegister(register, confirmation, &wg)
	}

	go generateCars(config, gasStations, dieselStations, lpgStations, electricStations)

	// Wait for all stations and registers to finish processing

	wg.Wait()
	printMetrics()
}
func generateCars(config Config, gasStations, dieselStations, lpgStations, electricStations []Station) {
	totalCars := config.Cars.Count
	werdun += totalCars
	for i := 0; i < totalCars; i++ {
		carType := getRandomCarType()
		arrivalTime := time.Now() // Pro jednoduchost nastavte čas příchodu na aktuální čas
		car := Car{ID: i, Type: carType, arrivalTime: arrivalTime}
		// Najít stanici odpovídající typu auta
		var stations []Station
		switch carType {
		case 1:
			stations = gasStations
			countRec["TotalCarGas"] += 1
		case 2:
			stations = dieselStations
			countRec["TotalCarDiesel"] += 1
		case 3:
			stations = lpgStations
			countRec["TotalCarLPG"] += 1
		case 4:
			stations = electricStations
			countRec["TotalCarElectric"] += 1
		}
		if len(stations) > 0 {
			// Vybrat náhodnou stanici daného typu auta
			for {
				station := stations[rand.Intn(len(stations))]
				if station.status {
					// Přidat auto do fronty stanice
					station.Queue <- &car
					countRec["TotalCars"] += 1
					break
				}
			}

		} else {
			fmt.Printf("No station available for car %d of type %d\n", car.ID, car.Type)
		}

		interval := randomDuration(parseDuration(config.Cars.ArrivalTimeMin), parseDuration(config.Cars.ArrivalTimeMax))
		time.Sleep(interval)

	}
}
func createStations(stTypes []StationType, stype int8) []Station {
	var stations []Station
	for idx, stType := range stTypes {
		for i := 0; i < stType.Count; i++ {
			stations = append(stations, Station{
				ID:           (int(stype)*10 + i + 1*(idx)), // Unique ID for each station of the same type
				Type:         stype,
				serveTimeMin: parseDuration(stType.ServeTimeMin),
				serveTimeMax: parseDuration(stType.ServeTimeMax),
				Queue:        make(chan *Car),
				status:       true,
			})
		}
	}
	return stations
}
func createRegisters(regConfig RegisterConfig) []Register {
	var registers []Register
	for i := 0; i < regConfig.Count; i++ {
		registers = append(registers, Register{
			ID:            i + 1, // Unique ID for each register
			HandleTimeMin: parseDuration(regConfig.HandleTimeMin),
			HandleTimeMax: parseDuration(regConfig.HandleTimeMax),
			Queue:         make(chan *Car),
		})
	}
	return registers
}
func processStation(station Station, registers []Register, confirmation chan *Car, wg *sync.WaitGroup, stats Stations, regs []Register) {
	defer wg.Done()

	for car := range station.Queue {
		// Simulace tankování
		car.station = &station
		tankTime := randomDuration(station.serveTimeMin, station.serveTimeMax)
		car.tankStartTime = time.Now()
		time.Sleep(tankTime)
		car.tankEndTime = time.Now()
		// Auto dokončilo tankování
		//fmt.Printf("Car %d started at %s, Tank time: %s and finished at %s --%s station%d. \n", car.ID, car.tankStartTime.Format("15:04:05"), tankTime, car.tankEndTime.Format("15:04:05"), getStationTypeName(station.Type), station.ID)

		// Odeslat auto na registr
		if len(registers) > 0 {
			// Vybrat kratší frontu
			var minLength int = len(registers[0].Queue)
			var minReg Register = registers[0]
			for _, register := range registers {
				if minLength < len(register.Queue) {
					minLength = len(register.Queue)
					minReg = register
				}
			}
			// Předat pointer na auto a kanál pro potvrzení
			minReg.Queue <- car
			// Čekat na potvrzení od registru
			<-confirmation
			fmt.Printf("Car %d type %d tanked at station%d.\n  arrived at %s, tanking from %s to %s, payed at %s and left at %s\n\n", car.ID, car.Type, station.ID, car.arrivalTime.Format("15:04:05"), car.tankStartTime.Format("15:04:05.000"), car.tankEndTime.Format("15:04:05.000"), car.payStart.Format("15:04:05.000"), car.leavingTime.Format("15:04:05.000"))
			werdun -= 1
			//Statistika času
			timeRec["TimeInQueue"] += car.tankStartTime.Sub(car.arrivalTime)
			timeRec["TimeToPay"] += car.leavingTime.Sub(car.payStart)
			fmt.Printf("%s - %s = %d\n", car.payStart.Format("15:04:05"), car.tankEndTime.Format("15:04:05"), car.payStart.Sub(car.tankEndTime).Milliseconds())
			var totalTime = car.leavingTime.Sub(car.arrivalTime)
			timeRec["TotalTime"] += totalTime
			switch car.Type {
			case 1:
				timeRec["TimeOnGas"] += totalTime
				timeRec["TimeInQueueGas"] += car.tankStartTime.Sub(car.arrivalTime)
				if timeRec["MinGasPass"] > totalTime {
					timeRec["MinGasPass"] = totalTime
				}
				if timeRec["MaxGasPass"] < totalTime {
					timeRec["MaxGasPass"] = totalTime
				}

			case 2:
				timeRec["TimeOnDiesel"] += totalTime
				timeRec["TimeInQueueDiesel"] += car.tankStartTime.Sub(car.arrivalTime)
				if timeRec["MinDieselPass"] > totalTime {
					timeRec["MinDieselPass"] = totalTime
				}
				if timeRec["MaxDieselPass"] < totalTime {
					timeRec["MaxDieselPass"] = totalTime
				}
			case 3:
				timeRec["TimeOnLPG"] += totalTime
				timeRec["TimeInQueueLPG"] += car.tankStartTime.Sub(car.arrivalTime)
				if timeRec["MinLPGPass"] > totalTime {
					timeRec["MinLPGPass"] = totalTime
				}
				if timeRec["MaxLPGPass"] < totalTime {
					timeRec["MaxLPGPass"] = totalTime
				}
			case 4:
				timeRec["TimeOnElectric"] += totalTime
				timeRec["TimeInQueueElectric"] += car.tankStartTime.Sub(car.arrivalTime)
				if timeRec["MinElectricPass"] > totalTime {
					timeRec["MinElectricPass"] = totalTime
				}
				if timeRec["MaxElectricPass"] < totalTime {
					timeRec["MaxElectricPass"] = totalTime
				}
			}
		}

		// Ukončení gorutin
		if werdun == 0 {
			for _, r := range regs {
				close(r.Queue)
			}
			fmt.Printf("zavvira stanice%d", station.ID)
			// Iterate over gas stations
			for _, s := range stats.gas {
				close((s.Queue))
			}
			for _, s := range stats.diesel {
				close((s.Queue))
			}
			for _, s := range stats.lpg {
				close((s.Queue))
			}
			for _, s := range stats.el {
				close((s.Queue))
			}

			close((confirmation))
		}
	}
}
func printMetrics() {
	fmt.Printf("\n\n               < Global statistics output >\n-------------------------------------------------------------\n")
	fmt.Printf("Total time spent at gas station: %d ms \n", timeRec["TotalTime"].Milliseconds())
	fmt.Printf("Total ammount of cars: %d \n", countRec["TotalCars"])
	fmt.Printf("Average time to pay: %f ms \n", (float64(timeRec["TimeToPay"]))/(float64)(countRec["TotalCars"])/(float64)(10000000))
	fmt.Printf("Total time spent in queue: %d ms \n", timeRec["TimeInQueue"].Milliseconds())
	fmt.Printf("\n\n                < Specific statistics >\n-------------------------------------------------------------\n")
	fmt.Printf("\n< Gas cars statistics >\n\n")
	fmt.Printf("Total ammount of cars: %d  \n", countRec["TotalCarGas"])
	fmt.Printf("Total time spent as gas car driver: %d ms \n", timeRec["TimeOnGas"].Milliseconds())
	fmt.Printf("Average time spent as gas car driver: %f ms \n", (float64(timeRec["TimeOnGas"]))/(float64)(countRec["TotalCarGas"])/(float64)(1000000))
	fmt.Printf("Average time in queue as gas car driver: %f ms \n", (float64(timeRec["TimeInQueueGas"]))/(float64)(countRec["TotalCarElectric"])/(float64)(1000000))
	fmt.Printf("The fastes service for: %d ms \n", timeRec["MinGasPass"].Milliseconds())
	fmt.Printf("The worst service for: %d ms \n", timeRec["MaxGasPass"].Milliseconds())

	fmt.Printf("\n< Diesel cars statistics >\n\n")
	fmt.Printf("Total ammount of cars: %d  \n", countRec["TotalCarDiesel"])
	fmt.Printf("Total time spent as diesel user: %d ms \n", timeRec["TimeOnDiesel"].Milliseconds())
	fmt.Printf("Average time spent as diesel car driver: %f ms \n", (float64(timeRec["TimeOnDiesel"]))/(float64)(countRec["TotalCarDiesel"])/(float64)(1000000))
	fmt.Printf("Average time in queue as diesel car driver: %f ms \n", (float64(timeRec["TimeInQueueDiesel"]))/(float64)(countRec["TotalCarElectric"])/(float64)(1000000))
	fmt.Printf("The fastes service for: %d ms \n", timeRec["MinDieselPass"].Milliseconds())
	fmt.Printf("The worst service for: %d ms \n", timeRec["MaxDieselPass"].Milliseconds())

	fmt.Printf("\n\n< LPG cars statistics >\n\n")
	fmt.Printf("Total ammount of cars: %d \n", countRec["TotalCarLPG"])
	fmt.Printf("Total time spent as LPG user: %d ms \n", timeRec["TimeOnLPG"].Milliseconds())
	fmt.Printf("Average time spent as LPG car driver: %f ms \n", (float64(timeRec["TimeOnLPG"]))/(float64)(countRec["TotalCarLPG"])/(float64)(1000000))
	fmt.Printf("Average time in queue as LPG car driver: %f ms \n", (float64(timeRec["TimeInQueueElectric"]))/(float64)(countRec["TotalCarElectric"])/(float64)(1000000))
	fmt.Printf("The fastes service for: %d ms \n", timeRec["MinLPGPass"].Milliseconds())
	fmt.Printf("The worst service for: %d ms \n", timeRec["MaxLPGPass"].Milliseconds())
	fmt.Printf("\n< Electric cars statistics >\n\n")
	fmt.Printf("Total ammount of cars: %d \n", countRec["TotalCarElectric"])
	fmt.Printf("Total time spent as electric user: %d ms \n", timeRec["TimeOnElectric"].Milliseconds())
	fmt.Printf("Average time spent as electric car driver: %f ms \n", (float64(timeRec["TimeOnElectric"]))/(float64)(countRec["TotalCarElectric"])/(float64)(1000000))
	fmt.Printf("Average time in queue as electric car driver: %f ms \n", (float64(timeRec["TimeInQueueElectric"]))/(float64)(countRec["TotalCarElectric"])/(float64)(1000000))
	fmt.Printf("The fastes service for: %d ms \n", timeRec["MinElectricPass"].Milliseconds())
	fmt.Printf("The worst service for: %d ms \n", timeRec["MaxElectricPass"].Milliseconds())

}
func processRegister(register Register, confirmation chan *Car, wg *sync.WaitGroup) {
	defer wg.Done()

	for car := range register.Queue {
		// Simulace obsluhy
		handleTime := randomDuration(register.HandleTimeMin, register.HandleTimeMax)
		car.payStart = time.Now()
		time.Sleep(handleTime)
		// Auto dokončilo obsluhu
		car.leavingTime = time.Now()
		//fmt.Printf("Car %d handled at register %d. od %s do %s\n", car.ID, register.ID, car.payStart.Format("15:04:05"), car.leavingTime.Format("15:04:05"))
		// Nastavit čas odjezdu

		// Odeslat potvrzení zpět na stanici
		confirmation <- car

	}
}
func getRandomCarType() int8 {
	return int8(rand.Intn(4) + 1) // Random type of car (1 to 4)
}
func parseDuration(durationStr string) time.Duration {
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		panic(err)
	}
	return duration
}
func randomDuration(min, max time.Duration) time.Duration {
	return min + time.Duration(rand.Intn(int(max-min+1)))
}
