#!/usr/bin/env python3
"""Exporta um backup SQLite somente para leitura; não altera o banco original."""
import argparse
import json
import os
import sqlite3
from pathlib import Path

parser = argparse.ArgumentParser(description='Exportar SQLite do Império Bárbaro CRM para migração.')
parser.add_argument('database', type=Path, help='Cópia consistente do banco SQLite')
parser.add_argument('output', type=Path, help='Arquivo JSON privado; deve ainda não existir')
args = parser.parse_args()
source = args.database.resolve()
if not source.is_file():
    parser.error('Banco não encontrado.')
db = sqlite3.connect(source.as_uri() + '?mode=ro', uri=True)
db.row_factory = sqlite3.Row
tables = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table'")}
contacts = []
if 'contact_people' in tables:
    for person in db.execute('SELECT id,real_name,phone,notes FROM contact_people ORDER BY id'):
        accounts = [dict(playerTag=a['player_tag'], playerName=a['player_name'], clanId=a['clan_id']) for a in db.execute('SELECT player_tag,player_name,clan_id FROM contact_accounts WHERE contact_id=? ORDER BY player_tag', (person['id'],))]
        contacts.append(dict(id=person['id'], realName=person['real_name'], phone=person['phone'], notes=person['notes'], accounts=accounts))
elif 'contacts' in tables:
    for person in db.execute('SELECT * FROM contacts ORDER BY id'):
        contacts.append(dict(id=person['id'], realName=person['real_name'].strip() or person['player_name'], phone=person['phone'], notes=person['notes'], accounts=[dict(playerTag='#'+person['player_tag'].strip().lstrip('#').upper(), playerName=person['player_name'], clanId='')]))
raids = []
if 'raid_reports' in tables:
    for row in db.execute('SELECT * FROM raid_reports ORDER BY clan_tag,start_time'):
        raids.append(dict(clanTag=row['clan_tag'], startTime=row['start_time'], season=json.loads(row['season']), roster=json.loads(row['roster']), rosterCapturedAt=row['roster_captured_at'], rosterDuringRaid=bool(row['roster_during_raid']), syncedAt=row['synced_at']))
db.close()
export = dict(version=1, contacts=contacts, raids=raids)
fd = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w', encoding='utf-8') as output:
    json.dump(export, output, ensure_ascii=False)
print(f'Exportação privada concluída: {len(contacts)} contatos e {len(raids)} raids. O original foi preservado.')
