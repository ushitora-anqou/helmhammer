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
    std.format('"%s"', std.strReplace(args[0], '"', '\\"')),

  squote(args):
    std.format("'%s'", std.strReplace(args[0], "'", "\\'")),

  not(args):
    !args[0],

  toYaml(args):
    std.manifestYamlDoc(args[0], quote_keys=false),

  tpl(args):
    local
      str = args[0],
      dot = args[1],
      loop(i, out, state) =
        local s = state.state;
        if i >= std.length(str) then
          if s == 0 then out
          else if s == 1 then out + '{'
          else error 'unexpected termination of template'
        else
          local c = str[i];
          if s == 0 then  // initial state; find "{{"
            if c == '{' then loop(i + 1, out, { state: 1 }) tailstrict
            else loop(i + 1, out + c, { state: 0 }) tailstrict
          else if s == 1 then  // found "{"; find next "{"
            if c == '{' then loop(i + 1, out, { state: 2 }) tailstrict
            else loop(i + 1, out + '{' + c, { state: 0 }) tailstrict
          else if s == 2 then error 'FIXME'
          else
            error 'unknown state'
    ;
    loop(0, '', { state: 0 }),

  chartMain(
    chartName,
    chartVersion,
    chartAppVersion,
    releaseName,
    releaseService,
    keys,
    defaultValues,
    files,
  ):
    function(values={})
      local aux(key) =
        std.parseYaml(files[key]({
          Values: std.mergePatch(defaultValues, values),
          Chart: {
            Name: chartName,
            Version: chartVersion,
            AppVersion: chartAppVersion,
          },
          Release: {
            Name: releaseName,
            Service: releaseService,
          },
        }));
      std.filter(function(x) x != null, std.map(aux, keys)),
};
// DON'T USE BELOW
assert
  helmhammer.tpl(['', {}]) == '' &&
  helmhammer.tpl(['abc', {}]) == 'abc' &&
  helmhammer.tpl(['{', {}]) == '{' &&
  helmhammer.tpl(['{ {', {}]) == '{ {' &&
  true
  ;
'ok'
