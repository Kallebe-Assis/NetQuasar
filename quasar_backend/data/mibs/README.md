# Pasta de MIBs locais

Coloque aqui subpastas com arquivos `.txt` e `.csv` contendo definições/OIDs de fabricantes.

Exemplo:

- `data/mibs/mikrotik/`
- `data/mibs/zte/`
- `data/mibs/huawei/`

No cadastro do equipamento, informe o caminho da pasta:

- relativo ao backend (ex.: `data/mibs/mikrotik`)
- ou caminho absoluto no disco.

Quando o caminho estiver preenchido, o SNMP Discovery também analisa todos os `.txt`/`.csv` dessa pasta para ajudar a selecionar os OIDs do equipamento.
