# StarTracker - SSV Operators Nodes Tracker

## Build

```
docker build -t stakestar/startracker:latest .
```

## Run

```
docker run stakestar/startracker --db-path=data/nodes.db --geodb-path=GeoLite2-City.mmdb
```