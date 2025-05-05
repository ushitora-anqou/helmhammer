local helmhammer = {
  field(receiver, fieldName, args):
    if !std.isObject(receiver) || !std.objectHas(receiver, fieldName) then
      if std.length(args) != 0 then error 'field: invalid arguments: %s: %s' % [fieldName, args[0]]
      else null
    else if std.isFunction(receiver[fieldName]) then receiver[fieldName](args)
    else if std.length(args) != 0 then error 'field: invalid arguments: %s: %s: %s' % [receiver, fieldName, args[0]]
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
    if values == null then felse(state { v: '' })
    else if std.isNumber(values) then
      self.range(state, std.makeArray(values, function(x) x), fthen, felse)
    else if std.isArray(values) then
      if std.length(values) == 0 then felse(state { v: '' })
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
            state: state { v: '' },
          },
        ).state
    else if std.isObject(values) then
      if std.length(values) == 0 then felse(state { v: '' })
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
            state: state { v: '' },
          },
        ).state
    else error 'range: not implemented',

  printf(args):
    std.format(args[0], args[1:]),

  include(args0):
    local templates = args0['$'], args = args0.args, vs = args0.vs;
    { v: templates[args[0]](args[1]), vs: vs },

  contains(args):
    std.findSubstr(args[0], args[1]) != [],

  default(args):
    assert std.length(args) >= 1;
    if std.length(args) == 1 || $.empty([args[1]]) then args[0]
    else args[1],

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
    std.join(
      ' ',
      std.map(
        function(x) '"%s"' % [std.strReplace(x, '"', '\\"')],
        std.filterMap(function(x) x != null, std.toString, args),
      ),
    ),

  squote(args):
    std.join(
      ' ',
      std.map(
        function(x) "'%s'" % [std.strReplace(x, "'", "\\'")],
        std.filterMap(function(x) x != null, std.toString, args),
      ),
    ),

  not(args):
    !$.isTrue(args[0]),

  or(args):
    assert std.length(args) >= 1;
    local loop(i) =
      if i == std.length(args) - 1 || $.isTrue(args[i]) then args[i]
      else loop(i + 1);
    loop(0),

  and(args):
    assert std.length(args) >= 1;
    local loop(i) =
      if i == std.length(args) - 1 || !$.isTrue(args[i]) then args[i]
      else loop(i + 1);
    loop(0),

  eq(args):
    assert std.length(args) == 2;
    args[0] == args[1],

  ne(args):
    assert std.length(args) == 2;
    args[0] != args[1],

  print(args):
    // Equivalent to fmt.Sprint of Go.
    //
    // > Sprint formats using the default formats for its operands and
    // > returns the resulting string. Spaces are added between operands
    // > when neither is a string.
    local aux(i, out) =
      if i >= std.length(args) then out
      else if std.isString(args[i]) then aux(i + 1, out + args[i])
      else if i >= 1 && !std.isString(args[i - 1]) then
        aux(i + 1, out + ' ' + std.toString(args[i]))
      else
        aux(i + 1, out + std.toString(args[i]));
    aux(0, ''),

  concat(args):
    std.join([], args),

  list(args):
    args,

  lower(args):
    assert std.length(args) == 1;
    std.asciiLower(args[0]),

  required(args):
    assert std.length(args) == 2;
    // FIXME
    if args[1] == null then error args[0],

  sha256sum(args):
    assert std.length(args) == 1;
    std.sha256(args[0]),

  toYaml(args):
    std.manifestYamlDoc(args[0], quote_keys=false),

  dir(args):
    assert std.length(args) == 1;
    std.join('/', std.split(args[0], '/')[0:-1]),

  toInt(v):
    if std.isNumber(v) then v
    else if std.isString(v) then std.parseInt(v)
    else error 'toInt: not number nor string',

  min(args):
    assert std.length(args) >= 1;
    std.minArray(std.map($.toInt, args)),

  empty(args):
    assert std.length(args) == 1;
    local v = args[0];
    v == null ||
    ((std.isArray(v) || std.isObject(v) || std.isString(v)) && std.length(v) == 0) ||
    (std.isBoolean(v) && !v) ||
    (std.isNumber(v) && v == 0),

  hasKey(args):
    assert std.length(args) == 2;
    assert std.isObject(args[0]);
    assert std.isString(args[1]);
    std.objectHas(args[0], args[1]),

  b64enc(args):
    assert std.length(args) == 1;
    assert std.isString(args[0]);
    std.base64(args[0]),

  dict(args):
    local loop(i, out) =
      if i >= std.length(args) then out
      else
        local key = std.toString(args[i]);
        if i + 1 >= std.length(args) then
          loop(i + 2, out { [key]: '' })
        else
          loop(i + 2, out { [key]: args[i + 1] });
    loop(0, {}),

  gt(args):
    assert std.length(args) == 2;
    args[0] > args[1],

  int(args):
    assert std.length(args) == 1;
    $.toInt(args[0]),

  toString(args):
    assert std.length(args) == 1;
    std.toString(args[0]),

  has(args):
    assert std.length(args) == 2;
    local needle = args[0], haystack = args[1];
    assert std.isArray(haystack);
    std.member(haystack, needle),

  tuple(args):
    $.list(args),

  fail(args):
    assert std.length(args) == 1;
    assert std.isString(args[0]);
    error ('fail: %s' % [args[0]]),

  index(args):
    assert std.length(args) >= 2;
    std.foldl(
      function(v, arg)
        if std.isObject(v) then
          if !std.isString(arg) then error 'index: key is not a string'
          else if std.objectHas(v, arg) then v[arg]
          else null
        else if std.isArray(v) then
          if !std.isNumber(arg) then error 'index: key is not an integer'
          else if arg < std.length(v) then v[arg]
          else null
        else null,
      args[1:],
      args[0],
    ),

  trimAll(args):
    assert std.length(args) == 2;
    assert std.isString(args[0]);
    assert std.isString(args[1]);
    local
      trimLeft(s, cutset) =
        local loop(i) =
          if i >= std.length(s) || !std.member(cutset, s[i]) then i
          else loop(i + 1);
        s[loop(0):],
      trimRight(s, cutset) =
        local loop(i) =
          if i < 0 || !std.member(cutset, s[i]) then i
          else loop(i - 1);
        s[0:loop(std.length(s) - 1) + 1],
      s = args[0],
      cutset = args[1],
      s1 = trimLeft(s, cutset),
      s2 = trimRight(s1, cutset);
    s2,

  parseYaml(src):
    // avoid a go-jsonnet's known issue:
    // https://github.com/google/go-jsonnet/issues/714
    if src == '' then null
    else std.parseYaml(src),

  fromYaml(args):
    assert std.length(args) == 1;
    assert std.isString(args[0]);
    $.parseYaml(args[0]),

  int64(args):
    assert std.length(args) == 1;
    local v = args[0];
    if v == null then 0
    else if std.isNumber(v) then v
    else if std.isString(v) then std.parseInt(v)
    else if std.isBoolean(v) then if v then 1 else 0
    else error 'int64: invalid type',

  deepCopy(args):
    assert std.length(args) == 1;
    args[0],

  trim(args):
    assert std.length(args) == 1;
    assert std.isString(args[0]);
    std.trim(args[0]),

  omit(args):
    assert std.length(args) >= 1;
    assert std.isObject(args[0]);
    std.foldl(std.objectRemoveKey, args[1:], args[0]),

  regexReplaceAll(args):
    // ["[^-A-Za-z0-9_.]", "v2.14.11", "-"]
    assert std.length(args) == 3;
    assert std.isString(args[0]);
    assert std.isString(args[1]);
    assert std.isString(args[2]);
    if args[0] == '[^-A-Za-z0-9_.]' && args[1] == 'v2.14.11' then
      'v2.14.11'
    else
      error ('regexReplaceAll: not implemented: %s' % [args]),

  mergeOverwrite(args):
    if std.length(args) == 2 && args[0] == {} && args[1] == {} then {}
    else error ('mergeOverwrite: not implemented: %s' % [args]),

  ternary(args): error 'ternary: not implemented',
  typeIs(args): error 'typeIs: not implemented',
  toRawJson(args): error 'toRawJson: not implemented',
  dateInZone(args): error 'dateInZone: not implemented',
  now(args): error 'now: not implemented',

  set(args): error 'set: not implemented',
  //set(vs, dname, key, value):
  //  assert std.isObject(vs[dname]);
  //  assert std.isString(key);
  //  local vs1 = vs { [dname]: vs[dname] { [key]: value } };
  //  [vs1, vs1[dname]],

  tpl_(templates):
    {
      local strIndex(pat, str, start) =
        // FIXME: slow
        local occurrences = std.findSubstr(pat, str[start:std.length(str)]);
        if occurrences == [] then -1 else start + occurrences[0],

      local isSpace(c) =
        c == ' ' || c == '\n' || c == '\r' || c == '\t',

      local findNonSpace(str, i, step) =
        local c = str[i];
        if i < 0 || i >= std.length(str) then
          i
        else if isSpace(c) then
          findNonSpace(str, i + step, step)
        else
          i,

      local lexText(str, i0, out, skipLeadingSpaces) =
        assert i0 < std.length(str) : 'lexText: unexpected eof';
        local i =
          if skipLeadingSpaces then findNonSpace(str, i0, 1)
          else i0;
        assert i < std.length(str) : 'lexText: unexpected eof';
        /*
                  0 1 2 3 4 5
                  a   { { - a
          i     = 0
          j     =     2
          j - 1 =   1
          j + 2 =         4
          j + 3 =           5
          k     = 0
          k + 1 =   1
        */
        local j = strIndex('{{', str, i);
        if j == -1 then out + [{ t: 'text', v: str[i:] }]
        else
          assert j + 2 < std.length(str) : 'lexText: unexpected {{';
          if str[j + 2] == '-' then
            local k = findNonSpace(str, j - 1, -1);
            lexInsideAction(
              str,
              j + 3,
              (if i >= k + 1 then out else out + [{ t: 'text', v: str[i:k + 1] }]) + [{ t: '{{' }]
            ) tailstrict
          else
            lexInsideAction(
              str,
              j + 2,
              (if i >= j then out else out + [{ t: 'text', v: str[i:j] }]) + [{ t: '{{' }]
            ) tailstrict,

      local isAlphanumeric(ch) =
        local c = std.codepoint(ch);
        ch == '_' ||
        std.codepoint('a') <= c && c <= std.codepoint('z') ||
        std.codepoint('A') <= c && c <= std.codepoint('Z') ||
        std.codepoint('0') <= c && c <= std.codepoint('9'),

      local isNumeric(ch) =
        local c = std.codepoint(ch);
        std.codepoint('0') <= c && c <= std.codepoint('9'),

      // lexFieldOrVariable scans a field or variable: [.$]Alphanumeric.
      // The . or $ has been scanned.
      local lexFieldOrVariable(str, i) =
        local
          loop(i) =
            if i >= std.length(str) then error 'lexFieldOrVariable: unexpected eof'
            else if isAlphanumeric(str[i]) then loop(i + 1) tailstrict
            else i,
          j = loop(i);
        [j, str[i:j]],

      local lexIdentifier(str, i) =
        local
          loop(i) =
            if i >= std.length(str) then error 'lexIdentifier: unexpected eof'
            else if isAlphanumeric(str[i]) then loop(i + 1) tailstrict
            else i,
          j = loop(i);
        [j, str[i:j]],

      local lexNumber(str, i) =
        local
          loop(i) =
            if i >= std.length(str) then error 'lexNumber: unexpected eof'
            else if isNumeric(str[i]) then loop(i + 1) tailstrict
            else i,
          j = loop(i);
        [j, std.parseInt(str[i:j])],

      local lexString(str, i) =  // FIXME: escape
        local
          loop(i) =
            if i >= std.length(str) then error 'lexString: unexpected eof'
            else if str[i] == '"' then i + 1
            else loop(i + 1) tailstrict,
          j = loop(i + 1);
        [j, str[i + 1:j - 1]],

      local lexInsideAction(str, i, out) =
        if i + 2 < std.length(str) && str[i] == '-' && str[i + 1] == '}' && str[i + 2] == '}' then
          lex(str, i + 3, out + [{ t: '}}' }], skipLeadingSpaces=true)
        else if i + 1 < std.length(str) && str[i] == '}' && str[i + 1] == '}' then
          lex(str, i + 2, out + [{ t: '}}' }])
        else
          local c = str[i];
          if c == '.' then
            local res = lexFieldOrVariable(str, i + 1), j = res[0], v = res[1];
            lexInsideAction(str, j, out + [{ t: 'field', v: v }]) tailstrict
          else if c == '$' then
            local res = lexFieldOrVariable(str, i + 1), j = res[0], v = res[1];
            lexInsideAction(str, j, out + [{ t: 'var', v: v }]) tailstrict
          else if c == '|' then
            lexInsideAction(str, i + 1, out + [{ t: '|' }]) tailstrict
          else if isSpace(c) then
            lexInsideAction(str, findNonSpace(str, i + 1, 1), out + [{ t: ' ' }]) tailstrict
          else if c == '"' then
            local res = lexString(str, i), j = res[0], v = res[1];
            lexInsideAction(str, j, out + [{ t: 'string', v: v }]) tailstrict
          else if isNumeric(c) then
            local res = lexNumber(str, i), j = res[0], v = res[1];
            lexInsideAction(str, j, out + [{ t: 'number', v: v }]) tailstrict
          else if isAlphanumeric(c) then
            local res = lexIdentifier(str, i), j = res[0], v = res[1];
            local token =
              if v == 'with' then { t: 'with' }
              else if v == 'if' then { t: 'if' }
              else if v == 'else' then { t: 'else' }
              else if v == 'end' then { t: 'end' }
              else { t: 'id', v: v };
            lexInsideAction(str, j, out + [token]) tailstrict
          else error 'lexInsideAction: unexpected char',

      local lex(str, i, out, skipLeadingSpaces=false) =
        if i >= std.length(str) then
          out
        else
          lexText(str, i, out, skipLeadingSpaces),

      local findNonSpaceToken(toks, i) =
        if toks[i].t == ' ' then i + 1
        else i,

      local parseTerm(toks, i0) =
        local i = findNonSpaceToken(toks, i0);
        local tok = toks[i];
        if tok.t == 'field' then
          [i + 1, { t: 'field', v: tok.v }]
        else if tok.t == 'var' then
          [i + 1, { t: 'var', v: tok.v }]
        else if tok.t == 'id' then
          [i + 1, { t: 'id', v: tok.v }]
        else if tok.t == 'number' then
          [i + 1, { t: 'number', v: tok.v }]
        else if tok.t == 'string' then
          [i + 1, { t: 'string', v: tok.v }]
        else error ('parseTerm: unexpected token: %s' % [tok.t]),

      local parseOperand(toks, i) =
        local res = parseTerm(toks, i), j = res[0], node = res[1];
        if toks[j].t == 'field' then
          local
            aux(i, out) =
              if i >= std.length(toks) || toks[i].t != 'field' then out
              else aux(i + 1, out + [toks[i].v]),
            fields = aux(j, []);
          [j + std.length(fields), { t: 'chain', v: [node, fields] }]
        else [j, node],

      local parseCommand(toks, i) =
        local loop(i0, operands) =
          local i = findNonSpaceToken(toks, i0);
          if toks[i].t == '}}' then
            [i, { t: 'command', v: operands }]
          else if toks[i].t == '|' then
            [i + 1, { t: 'command', v: operands }]
          else
            local res = parseOperand(toks, i), j = res[0], node = res[1];
            loop(j, operands + [node]);
        loop(i, []),

      local parsePipeline(toks, i) =
        local loop(i0, commands) =
          local i = findNonSpaceToken(toks, i0);
          if toks[i].t == '}}' then [i + 1, { t: 'pipeline', v: commands }]
          else
            local res = parseCommand(toks, i), j = res[0], node = res[1];
            loop(j, commands + [node]);
        loop(i, []),

      local parseControl(toks, i) =
        local res = parsePipeline(toks, i), j = res[0], pipe = res[1];
        local res = parseList(toks, j), k0 = res[0], list = res[1];
        local
          res =
            local k1 = findNonSpaceToken(toks, k0), k2 = findNonSpaceToken(toks, k1 + 1);
            if toks[k1].t == 'else' && toks[k2].t == '}}' then parseList(toks, k2 + 1)
            else [k0, null],
          l0 = res[0],
          elseList = res[1];
        local l1 = findNonSpaceToken(toks, l0), l2 = findNonSpaceToken(toks, l1 + 1);
        if toks[l1].t != 'end' || toks[l2].t != '}}' then
          error 'parseControl: end not found'
        else
          [l2 + 1, { pipe: pipe.v, list: list, elseList: elseList }],

      local parseList(toks, i) =
        local loop(i, root) =
          if i >= std.length(toks) then
            [i, root]
          else
            local tok = toks[i];
            if tok.t == 'text' then
              loop(i + 1, root { v+: [{ t: 'text', v: tok.v }] }) tailstrict
            else if tok.t == '{{' then
              local i0 = i;
              local i = findNonSpaceToken(toks, i0 + 1);
              local tok = toks[i];
              if tok.t == 'with' || tok.t == 'if' then
                local res = parseControl(toks, i + 1), j = res[0], node = res[1];
                loop(j, root { v+: [{ t: tok.t, v: node }] }) tailstrict
              else if tok.t == 'else' || tok.t == 'end' then
                [i, root]
              else
                local res = parsePipeline(toks, i), j = res[0], node = res[1];
                loop(j, root { v+: [{ t: 'action', v: node }] }) tailstrict;
        loop(i, { t: 'list', v: [] }),

      local parse(toks/* tokens */, i) =
        local res = parseList(toks, i), j = res[0], node = res[1];
        if j < std.length(toks) then error 'parse: unexpected end'
        else node,

      local evalOperand(op, s0) =
        if op.t == 'chain' then
          local res = evalOperand(op.v[0], s0), s = res[0], val = res[1];
          [s, std.foldl(function(acc, field) acc[field], op.v[1], val)]
        else if op.t == 'field' then
          [s0, if op.v == '' then s0.dot else s0.dot[op.v]]
        else if op.t == 'var' then
          [s0, s0.vars[op.v]]
        else if op.t == 'number' || op.t == 'string' then
          [s0, op.v]
        else
          error 'evalOperand: unknown operand',

      local evalCommand(command, final, s0) =
        local op0 = command.v[0];  // FIXME
        if op0.t == 'id' then
          if op0.v == 'nindent' then
            local res = evalOperand(command.v[1], s0), s = res[0], val = res[1];
            [s, $.nindent([val, final])]
          else if op0.v == 'include' then
            local res = evalOperand(command.v[1], s0), s1 = res[0], name = res[1];
            local res = evalOperand(command.v[2], s1), s2 = res[0], newDot = res[1];
            [s2, $.include({ '$': templates, args: [name, newDot], vs: {} }).v]
          else if op0.v == 'tpl' then
            local res = evalOperand(command.v[1], s0), s1 = res[0], name = res[1];
            local res = evalOperand(command.v[2], s1), s2 = res[0], newDot = res[1];
            [s2, $.tpl({ '$': templates, args: [name, newDot], vs: {} }).v]
          else
            error ('evalCommand: unknown id: %s' % [op0.v])
        else
          evalOperand(op0, s0),

      local evalPipeline(commands, s0) =
        local acc =
          std.foldl(
            function(acc, command)
              local s0 = acc.s, final = acc.final;
              local res = evalCommand(command, final, s0), s1 = res[0], v = res[1];
              { s: s1, final: v },
            commands,
            { s: s0, final: null },
          );
        [acc.s, if acc.final == null then '' else acc.final],

      local eval(node, s0) =
        if node.t == 'text' then
          s0 { out+: node.v }
        else if node.t == 'list' then
          std.foldl(function(s, node) eval(node, s), node.v, s0)
        else if node.t == 'action' then
          assert node.v.t == 'pipeline';
          local res = evalPipeline(node.v.v, s0), s = res[0], val = res[1];
          s { out+: std.toString(val) }
        else if node.t == 'with' || node.t == 'if' then
          local res = evalPipeline(node.v.pipe, s0), s = res[0], pipeVal = res[1];
          if $.isTrue(pipeVal) then
            eval(node.v.list, if node.t == 'if' then s else s { dot: pipeVal })
          else if node.v.elseList != null then
            eval(node.v.elseList, s)
          else
            s0
        else error 'eval: unexpected node',

      strIndex: strIndex,
      findNonSpace: findNonSpace,
      lex: lex,
      parse: parse,
      eval: eval,
    },

  tpl(args0):
    local templates = args0['$'], args = args0.args, vs = args0.vs;
    local tpl_ = self.tpl_(templates), src = args[0], dot = args[1];
    {
      v: tpl_.eval(
        tpl_.parse(
          tpl_.lex(src, 0, []),
          0,
        ),
        {
          dot: dot,
          out: '',
          vars: { ''/* $ */: dot },
        },
      ).out,
      vs: vs,
    },

  chartMain(
    chartName,
    chartVersion,
    chartAppVersion,
    releaseName,
    releaseService,
    templateBasePath,
    capabilities,
    keys,
    defaultValues,
    crds,
    files,
  ):
    function(values={}, namespace='default', includeCrds=false)
      local
        runFile(key) =
          files[key]({
            Values: std.mergePatch(defaultValues, values),
            Chart: {
              Name: chartName,
              Version: chartVersion,
              AppVersion: chartAppVersion,
            },
            Release: {
              Name: releaseName,
              Namespace: namespace,
              Service: releaseService,
            },
            Template: {
              Name: key,
              BasePath: templateBasePath,
            },
            Capabilities: capabilities {
              APIVersions: {  // FIXME: APIVersions should behave as an array, too.
                Has(args):
                  assert std.length(args) == 1;
                  assert std.isString(args[0]);
                  // FIXME: support resource name like "apps/v1/Deployment"
                  std.member(capabilities.APIVersions, args[0]),
              },
            },
          }),
        flatten(ary) =
          local loop(i, out) =
            if i >= std.length(ary) then out
            else if std.isArray(ary[i]) then loop(i + 1, out + ary[i])
            else loop(i + 1, out + [ary[i]]);
          loop(0, []),
        parseManifests(src) =
          local manifests = std.join(
            '\n---\n',
            std.map(
              std.trim,
              std.split(
                if std.startsWith(src, '---') then src[3:] else src,
                '\n---',
              ),
            ),
          );
          $.parseYaml(manifests);
      std.filter(
        function(x) x != null,
        flatten(
          std.map(
            parseManifests,
            (if includeCrds then crds else []) + std.map(runFile, keys),
          ),
        ),
      ),
};
// DON'T USE BELOW

assert helmhammer.or([0, 0]) == 0;
assert helmhammer.or([1, 0]) == 1;
assert helmhammer.or([0, true]) == true;
assert helmhammer.or([1, 1]) == 1;

assert helmhammer.and([false, 0]) == false;
assert helmhammer.and([1, 0]) == 0;
assert helmhammer.and([0, true]) == 0;
assert helmhammer.and([1, 1]) == 1;

assert helmhammer.dir(['/run/topolvm/lvmd.sock']) == '/run/topolvm';

assert helmhammer.index([
  [0, [0, 0, [0, 0, 0, 1]]],
  1,
  2,
  3,
]) == 1;

assert helmhammer.trimAll(['aabbcc', 'ac']) == 'bb';

local tpl_ = helmhammer.tpl_({});
assert tpl_.strIndex('', '', 0) == -1;
assert tpl_.strIndex('a', '', 0) == -1;
assert tpl_.strIndex('', 'a', 0) == -1;
assert tpl_.strIndex('a', 'a', 0) == 0;
assert tpl_.strIndex('b', 'a', 0) == -1;
assert tpl_.strIndex('a', 'a', 1) == -1;
assert tpl_.strIndex('a', 'aa', 1) == 1;
assert tpl_.strIndex('aa', 'baa', 1) == 1;
assert tpl_.findNonSpace(' a', 0, 1) == 1;
assert tpl_.findNonSpace('a ', 1, -1) == 0;
assert tpl_.findNonSpace(' ', 0, -1) == -1;
assert tpl_.findNonSpace(' ', 0, 1) == 1;
assert tpl_.lex('aa', 0, []) == [{ t: 'text', v: 'aa' }];
assert tpl_.lex('{{}}', 0, []) == [{ t: '{{' }, { t: '}}' }];
assert tpl_.lex('a{{}}', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: '}}' }];
assert tpl_.lex('a {{}}', 0, []) == [{ t: 'text', v: 'a ' }, { t: '{{' }, { t: '}}' }];
assert tpl_.lex('{{- }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl_.lex('a{{- }}', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl_.lex('a {{- }}', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl_.lex('{{ -}}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl_.lex('{{ -}}a', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('{{ -}} a', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('{{- -}}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl_.lex('a{{- -}}a', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a {{- -}}a', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a{{- -}} a', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a {{- -}} a', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a{{}}b', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: '}}' }, { t: 'text', v: 'b' }];
assert tpl_.lex('{{ . }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: 'field', v: '' }, { t: ' ' }, { t: '}}' }];
assert tpl_.lex('{{ .A }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: 'field', v: 'A' }, { t: ' ' }, { t: '}}' }];
assert tpl_.lex('{{ .A.b }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: 'field', v: 'A' }, { t: 'field', v: 'b' }, { t: ' ' }, { t: '}}' }];
assert tpl_.lex('{{ .A.b }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: 'field', v: 'A' }, { t: 'field', v: 'b' }, { t: ' ' }, { t: '}}' }];
assert tpl_.parse(tpl_.lex('', 0, []), 0) == { t: 'list', v: [] };
assert tpl_.parse(tpl_.lex('a', 0, []), 0) == { t: 'list', v: [{ t: 'text', v: 'a' }] };
assert tpl_.parse(tpl_.lex('a{{}}b', 0, []), 0) == {
  t: 'list',
  v: [
    { t: 'text', v: 'a' },
    { t: 'action', v: { t: 'pipeline', v: [] } },
    { t: 'text', v: 'b' },
  ],
};
assert tpl_.parse(tpl_.lex('a{{.}}b', 0, []), 0) == { t: 'list', v: [
  { t: 'text', v: 'a' },
  { t: 'action', v: { t: 'pipeline', v: [
    { t: 'command', v: [{ t: 'field', v: '' }] },
  ] } },
  { t: 'text', v: 'b' },
] };

local tpl(args) =
  helmhammer.tpl({ '$': { tpl0(dot): dot.valueTpl0 }, args: args, vs: {} }).v;
assert tpl(['', {}]) == '';
assert tpl(['a', {}]) == 'a';
assert tpl(['{', {}]) == '{';
assert tpl(['{ {', {}]) == '{ {';
assert tpl(['a{{}}b', {}]) == 'ab';
assert tpl(['a{{.}}b', 3]) == 'a3b';
assert tpl(['a{{.A}}b', { A: 3 }]) == 'a3b';
assert tpl(['a{{.A.b}}b', { A: { b: 'c' } }]) == 'acb';
assert tpl(['a{{.A.b}}{{.A.b}}b', { A: { b: 'c' } }]) == 'accb';
assert tpl(['a{{.A.b | nindent 1}}b', { A: { b: 'c' } }]) == 'a\n cb';
assert tpl(['a{{.A.b | nindent 1 | nindent 1}}b', { A: { b: 'c' } }]) == 'a\n \n  cb';
assert tpl(['a{{$}}b', 3]) == 'a3b';
assert tpl(['a{{$.A}}b', { A: 3 }]) == 'a3b';
assert tpl(['a{{$.A.b}}b', { A: { b: 'c' } }]) == 'acb';
assert tpl(['{{ include "tpl0" $ }}', { valueTpl0: 'here' }]) == 'here';
assert tpl(['{{ include "tpl0" . }}', { valueTpl0: 'here' }]) == 'here';
assert tpl(['>{{ with $ }}1{{ end }}<', true]) == '>1<';
assert tpl(['>{{ with $ }}1{{ end }}<', false]) == '><';
assert tpl(['{{ with .A }}{{.B}}{{ end }}', { A: { B: 1 } }]) == '1';
assert tpl(['>{{ with $ }}1{{ else }}0{{ end }}<', true]) == '>1<';
assert tpl(['>{{ with $ }}1{{ else }}0{{ end }}<', false]) == '>0<';
assert tpl(['>{{ if $ }}1{{ end }}<', true]) == '>1<';
assert tpl(['>{{ if $ }}1{{ end }}<', false]) == '><';
assert tpl(['{{ if .A }}{{.B}}{{ end }}', { A: { B: 1 }, B: 0 }]) == '0';
assert tpl(['>{{ if $ }}1{{ else }}0{{ end }}<', true]) == '>1<';
assert tpl(['>{{ if $ }}1{{ else }}0{{ end }}<', false]) == '>0<';
assert tpl(['{{ tpl "{{.A}}" $ }}', { A: 10 }]) == '10';
'ok'
