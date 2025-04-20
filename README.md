# Helmhammer

A work-in-progress compiler from a Helm Chart into Jsonnet.

## Design

Some examples of compilation:

Case 1:

```
{{$x := 2}}{{$x = 3}}{{$x}}

local
    s1 = { v: "", vs: s0.vs + { x: 2 } },
    s2 = { v: "", vs: s1.vs + { x: 3 } },
    s3 = { v: s2.vs.x, vs: s2.vs }
;
{
    v: helmhammer.join([s1.v, s2.v, s3.v]),
    vs: s3.vs,
}
```

Case 2:

```
{{$x := 2}}{{if true}}{{$x = 3}}{{end}}{{$x}}

local
    s1 = { v: "", vs: s0.vs + { x: 2 } },
    s2 =
        if true then
            local s4 = { v: "", vs: s1.vs + { x: 3 }};
            { v: s4.v, vs: s1.vs + { x: s4.x }}
        else
            { v: "", vs: s1.vs },
    s3 = { v: s2.vs.x, vs: s2.vs }
;
{
    v: helmhammer.join([s1.v, s2.v, s3.v]),
    vs: s3.vs,
}
```

Case 3:

```
{{$i := 0}}{{range $i = .}}{{$i}}{{end}}

local
    s1 = { v: "", vs: s0.vs + { i: 0 } },
    s2 =
        std.foldl(
            function(s3, v)
                local
                    s4 = { v: s3.v, vs: s3.vs + { i: v } },
                    s5 = { v: s4.i, vs: s4.vs }
                ;
                {
                    v: helmhammer.join([s4.v, s5.v]),
                    vs: s5.vs,
                },
            helmhammer.dot(),
            {
                v: "",
                vs: s1.vs,
            },
        )
    ;
{
    v: helmhammer.join([s1.v, s2.v]),
    vs: s2.vs,
}
```
