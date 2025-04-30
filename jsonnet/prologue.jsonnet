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

  tpl_(templates):
    {
      local strIndex(pat, str, start) =
        // FIXME: slow
        local occurrences = std.findSubstr(pat, str[start:std.length(str)]);
        if occurrences == [] then -1 else start + occurrences[0],

      local findNonSpace(str, i, step) =
        local c = str[i];
        if i < 0 || i >= std.length(str) then
          i
        else if c == ' ' || c == '\n' || c == '\r' || c == '\t' then
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
              if i >= k + 1 then out else out + [{ t: 'text', v: str[i:k + 1] }]
            ) tailstrict
          else
            lexInsideAction(
              str,
              j + 2,
              if i >= j then out else out + [{ t: 'text', v: str[i:j] }]
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
          else if c == ' ' then
            lexInsideAction(str, i + 1, out) tailstrict
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
              else if v == 'end' then { t: 'end' }
              else { t: 'id', v: v };
            lexInsideAction(str, j, out + [token]) tailstrict
          else error 'lexInsideAction: unexpected char',

      local lex(str, i, out, skipLeadingSpaces=false) =
        if i >= std.length(str) then
          out
        else
          lexText(str, i, out, skipLeadingSpaces),

      local parseTerm(toks, i) =
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
        local loop(i, operands) =
          if toks[i].t == '}}' then
            [i, { t: 'command', v: operands }]
          else if toks[i].t == '|' then
            [i + 1, { t: 'command', v: operands }]
          else
            local res = parseOperand(toks, i), j = res[0], node = res[1];
            loop(j, operands + [node]);
        loop(i, []),

      local parsePipeline(toks, i) =
        local loop(i, commands) =
          if toks[i].t == '}}' then [i + 1, { t: 'pipeline', v: commands }]
          else
            local res = parseCommand(toks, i), j = res[0], node = res[1];
            loop(j, commands + [node]);
        loop(i, []),

      local parseControl(toks, i) =
        local res = parsePipeline(toks, i), j = res[0], pipe = res[1];
        local res = parseList(toks, j), k = res[0], list = res[1];
        if toks[k].t != 'end' || toks[k + 1].t != '}}' then
          error 'parseControl: end not found'
        else
          [k + 2, { pipe: pipe.v, list: list }],

      local parseList(toks, i) =
        local loop(i, root) =
          if i >= std.length(toks) then
            [i, root]
          else
            local tok = toks[i];
            if tok.t == 'text' then
              loop(i + 1, root { v+: [{ t: 'text', v: tok.v }] }) tailstrict
            else if tok.t == 'with' then
              local res = parseControl(toks, i + 1), j = res[0], node = res[1];
              loop(j, root { v+: [{ t: 'with', v: node }] }) tailstrict
            else if tok.t == 'end' then
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
            [s2, $.include(templates)([name, newDot])]
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
        else if node.t == 'with' then
          local res = evalPipeline(node.v.pipe, s0), s = res[0], pipeVal = res[1];
          if $.isTrue(pipeVal) then eval(node.v.list, s { dot: pipeVal })
          else s0
        else error 'eval: unexpected node',

      strIndex: strIndex,
      findNonSpace: findNonSpace,
      lex: lex,
      parse: parse,
      eval: eval,
    },

  tpl(templates):
    function(args)
      local tpl_ = self.tpl_(templates), src = args[0], dot = args[1];
      tpl_.eval(
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
assert tpl_.lex('{{}}', 0, []) == [{ t: '}}' }];
assert tpl_.lex('a{{}}', 0, []) == [{ t: 'text', v: 'a' }, { t: '}}' }];
assert tpl_.lex('a {{}}', 0, []) == [{ t: 'text', v: 'a ' }, { t: '}}' }];
assert tpl_.lex('{{- }}', 0, []) == [{ t: '}}' }];
assert tpl_.lex('a{{- }}', 0, []) == [{ t: 'text', v: 'a' }, { t: '}}' }];
assert tpl_.lex('a {{- }}', 0, []) == [{ t: 'text', v: 'a' }, { t: '}}' }];
assert tpl_.lex('{{ -}}', 0, []) == [{ t: '}}' }];
assert tpl_.lex('{{ -}}a', 0, []) == [{ t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('{{ -}} a', 0, []) == [{ t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('{{- -}}', 0, []) == [{ t: '}}' }];
assert tpl_.lex('a{{- -}}a', 0, []) == [{ t: 'text', v: 'a' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a {{- -}}a', 0, []) == [{ t: 'text', v: 'a' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a{{- -}} a', 0, []) == [{ t: 'text', v: 'a' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a {{- -}} a', 0, []) == [{ t: 'text', v: 'a' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a{{}}b', 0, []) == [{ t: 'text', v: 'a' }, { t: '}}' }, { t: 'text', v: 'b' }];
assert tpl_.lex('{{ . }}', 0, []) == [{ t: 'field', v: '' }, { t: '}}' }];
assert tpl_.lex('{{ .A }}', 0, []) == [{ t: 'field', v: 'A' }, { t: '}}' }];
assert tpl_.lex('{{ .A.b }}', 0, []) == [{ t: 'field', v: 'A' }, { t: 'field', v: 'b' }, { t: '}}' }];
assert tpl_.lex('{{ .A.b }}', 0, []) == [{ t: 'field', v: 'A' }, { t: 'field', v: 'b' }, { t: '}}' }];
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

local tpl = helmhammer.tpl({ tpl0(dot): dot.valueTpl0 });
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
assert tpl(['>{{ with $ }}1{{ end }}<', true]) == '>1<';
assert tpl(['>{{ with $ }}1{{ end }}<', false]) == '><';
assert tpl(['{{ with .A }}{{.B}}{{ end }}', { A: { B: 1 } }]) == '1';
'ok'
