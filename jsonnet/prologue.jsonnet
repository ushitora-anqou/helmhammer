local helmhammer = {
  field(receiver, fieldName, args):
    if std.isFunction(receiver[fieldName]) then receiver[fieldName](args)
    else receiver[fieldName],

  join(ary):
    std.join('', std.map(std.toString, ary)),

  isTrue(v):
    if v == null then false
    else if std.isArray(v) || std.isObject(v) || std.isString(v) then std.length(v) > 0
    else if std.isBoolean(v) then v
    else if std.isFunction(v) then v != null
    else if std.isNumber(v) then v != 0
    else true,

  range(state, values, fthen, felse):
    if values == null then felse(state)
    else if std.isNumber(values) then
      self.range(state, std.makeArray(values, function(x) x), fthen, felse)
    else if std.isArray(values) then
      if std.length(values) == 0 then felse(state)
      else
        std.foldl(
          function(acc, value)
            local postState = fthen(acc.state, acc.i, value);
            {
              i: acc.i + 1,
              state: {
                v: acc.state.v + postState.v,
                vs: postState.vs,
              },
            },
          values,
          {
            i: 0,
            state: state,
          },
        ).state
    else if std.isObject(values) then
      if std.length(values) == 0 then felse(state)
      else
        std.foldl(
          function(acc, kv)
            local postState = fthen(acc.state, kv.key, kv.value);
            {
              i: acc.i + 1,
              state: {
                v: acc.state.v + postState.v,
                vs: postState.vs,
              },
            },
          std.objectKeysValues(values),
          {
            i: 0,
            state: state,
          },
        ).state
    else error 'range: not implemented',

  printf(args):
    std.format(args[0], args[1:]),

  include(root):
    function(args)
      root[args[0]](args[1]),

  contains(args):
    std.findSubstr(args[0], args[1]) != [],

  default(args):
    local v = args[0];
    if
      v == null ||
      std.isNumber(v) && v == 0 ||
      std.isString(v) && v == '' ||
      std.isArray(v) && v == [] ||
      std.isObject(v) && v == {} ||
      std.isBoolean(v) && v == false
    then
      args[1]
    else
      v,

  trimSuffix(args):
    if std.endsWith(args[1], args[0]) then
      std.substr(args[1], 0, std.length(args[1]) - std.length(args[0]))
    else
      args[1],

  trunc(args):
    if args[0] >= 0 then
      std.substr(args[1], 0, args[0])
    else
      std.substr(args[1], std.length(args[1]) + args[0], -args[0]),

  nindent(args):
    '\n' + $.indent(args),

  indent(args):
    std.join(
      '\n',
      std.map(
        function(x) std.repeat(' ', args[0]) + x,
        std.split(args[1], '\n'),
      ),
    ),

  replace(args):
    std.strReplace(args[2], args[0], args[1]),

  quote(args):
    std.format('"%%s"', std.strReplace(args[0], '"', '\\"')),

  squote(args):
    std.format("'%%s'", std.strReplace(args[0], "'", "\\'")),

  not(args):
    !args[0],

  toYaml(args):
    std.manifestYamlDoc(args[0], quote_keys=false),

  chartMain(keys, defaultValues, files):
    function(values)
      local aux(key) =
        std.parseYaml(files[key]({
          Values: std.mergePatch(defaultValues, values),
          Chart: {
            Name: 'hello',
            Version: '0.1.0',
            AppVersion: '1.16.0',
          },
          Release: {
            Name: 'hello',
            Service: 'Helm',
          },
        }));
      std.filter(function(x) x != null, std.map(aux, keys)),
};
''
