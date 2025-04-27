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

  // WIP; written by humans (anqou)
  //  tpl(args):
  //    local
  //      str = args[0],
  //      dot = args[1],
  //      loop(i, out, state) =
  //        local s = state.state;
  //        if i >= std.length(str) then
  //          if s == 0 then out
  //          else if s == 1 then out + '{'
  //          else error 'unexpected termination of template'
  //        else
  //          local c = str[i];
  //          if s == 0 then  // initial state; find "{{"
  //            if c == '{' then loop(i + 1, out, state { state: 1 }) tailstrict
  //            else loop(i + 1, out + c, state { state: 0 }) tailstrict
  //          else if s == 1 then  // found "{"; find next "{"
  //            if c == '{' then loop(i + 1, out, state { state: 2 }) tailstrict
  //            else loop(i + 1, out + '{' + c, state { state: 0 }) tailstrict
  //          else if s == 2 then  // found "{{"; check "-" is followed
  //            if c == '-' then loop(i + 1, out, state { state: 3, prefixMinus: true }) tailstrict
  //            else loop(i, out, state { state: 3 }) tailstrict
  //          else if s == 3 then  // found "{{" or "{{-"; skip spaces
  //            if c == ' ' then loop(i + 1, out, state { state: 3 }) tailstrict
  //            else loop(i, out, state { state: 4 }) tailstrict
  //          else if s == 4 then  // start to parse pipeline; eat '.'
  //            if c == '.' then loop(i + 1, out + dot, state {
  //            error 'FIXME'
  //          else
  //            error 'unknown state'
  //    ;
  //    loop(0, '', { state: 0 }),

  tpl(args):  // Written by ChatGPT
    local lib =
      {
        render(template, data)::
          self._render(template, data),

        _render(template, data):
          if self._findFirst('{{', template) == -1 then
            template
          else
            local block = self._parseNextBlock(template);
            local renderedInner =
              if block.kind == 'var' then
                self._evalVar(block.expr, data)
              else if block.kind == 'if' then
                self._evalIf(block, data)
              else if block.kind == 'range' then
                self._evalRange(block, data)
              else
                error 'unknown block kind: ' + block.kind;
            block.before + renderedInner + self._render(block.after, data),

        _parseNextBlock(template):
          local startIdx = self._findFirst('{{', template);
          local endIdx = self._findFirst('}}', template);
          local before = template[:startIdx];
          local inside = std.stripChars(template[startIdx + 2:endIdx], ' ');
          local after = template[endIdx + 2:];
          if std.startsWith(inside, 'if ') then
            self._parseIf(before, inside[3:], after)
          else if std.startsWith(inside, 'range ') then
            self._parseRange(before, inside[6:], after)
          else if inside == 'else' || inside == 'end' then
            error "unexpected 'else' or 'end' without matching block"
          else
            { kind: 'var', expr: inside, before: before, after: after },

        _extractIfBlock(template):
          local elseIdx = self._findFirst('{{else}}', template);
          local endIdx = self._findFirst('{{end}}', template);
          if endIdx == -1 then
            error 'unterminated if block'
          else if elseIdx != -1 && elseIdx < endIdx then
            {
              thenPart: template[:elseIdx],
              elsePart: template[elseIdx + 8:endIdx],  // 8 = len("{{else}}")
              remain: template[endIdx + 7:],  // 7 = len("{{end}}")
            }
          else
            {
              thenPart: template[:endIdx],
              elsePart: null,
              remain: template[endIdx + 7:],
            },

        _parseIf(before, condExpr, after):
          local block = self._extractIfBlock(after);
          {
            kind: 'if',
            cond: condExpr,
            thenPart: block.thenPart,
            elsePart: block.elsePart,
            before: before,
            after: block.remain,
          },

        _parseRange(before, listExpr, after):
          local block = self._extractBlock(after, ['else', 'end']);
          local parts = std.split(block.inner, '{{else}}');
          {
            kind: 'range',
            listExpr: listExpr,
            thenPart: parts[0],
            elsePart: if std.length(parts) == 2 then parts[1] else null,
            before: before,
            after: block.remain,
          },

        _extractBlock(template, endTags):
          local endIdx = std.foldl(
            function(acc, tag)
              local idx = self._findFirst('{{' + tag + '}}', template);
              if acc == -1 || (idx != -1 && idx < acc) then idx else acc,
            endTags,
            -1
          );
          if endIdx == -1 then error 'unterminated block'
          else {
            inner: template[:endIdx],
            remain: template[endIdx + std.length('{{end}}'):],
          },

        _evalVar(expr, data):
          local v = self._evalExpr(expr, data);
          std.toString(v),

        _evalIf(block, data):
          if self._truthy(self._evalExpr(block.cond, data)) then
            self._render(block.thenPart, data)
          else if block.elsePart != null then
            self._render(block.elsePart, data)
          else
            '',

        _evalRange(block, data):
          local list = self._evalExpr(block.listExpr, data);
          if list == null || std.length(list) == 0 then
            if block.elsePart != null then self._render(block.elsePart, data) else ''
          else
            std.join('', [self._render(block.thenPart, data { '.': item }) for item in list]),

        _evalExpr(expr, data):
          if std.startsWith(expr, '.') then
            self._lookupField(data, std.split(expr[1:], '.'))
          else
            data[expr],

        _lookupField(obj, path):
          if std.length(path) == 0 then obj
          else
            if std.objectHas(obj, path[0]) then
              self._lookupField(obj[path[0]], path[1:])
            else
              error 'field not found: ' + path[0],

        _truthy(x):
          if x == null then false
          else if x == false then false
          else if x == 0 then false
          else if x == '' then false
          else true,

        _findFirst(pat, str):
          local matches = std.findSubstr(pat, str);
          if std.length(matches) > 0 then matches[0] else -1,
      };
    lib.render(args[0], args[1]),

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
  helmhammer.tpl(['{{.A}}', { A: 'hello' }]) == 'hello' &&
  helmhammer.tpl(['{{.A}}{{.A}}', { A: 'hello' }]) == 'hellohello' &&
  helmhammer.tpl(['{{.A.B}}', { A: { B: 'hello' } }]) == 'hello' &&
  helmhammer.tpl(['{{if .C}}{{.A.B}}{{end}}', { A: { B: 'hello' }, C: true }]) == 'hello' &&
  helmhammer.tpl(['{{if .C}}{{.A.B}}{{end}}', { A: { B: 'hello' }, C: false }]) == '' &&
  helmhammer.tpl([
    '{{if .C}}{{.A.B}}{{else}}no{{end}}',
    { A: { B: 'hello' }, C: false },
  ]) == 'no' &&
  true
  ;
'ok'
