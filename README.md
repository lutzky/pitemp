# Live building

On build machine:

```
ls * | entr -c -s "./build.sh && scp build/main.arm TARGET-MACHINE:pitemp.new"
```

On pi, after first build:

```
ls pitemp.new | sudo entr -r -c -s "cp pitemp.new pitemp && exec ./pitemp"
```