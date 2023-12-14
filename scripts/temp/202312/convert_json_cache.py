import sqlite3
import os
import json
import datetime

import regex

db = sqlite3.connect(os.path.expanduser("~/.cache/kdcache/kd_data.db"))

cursor = db.cursor()

for fname in os.listdir(os.path.expanduser("~/.cache/kdcache/words/")):
    with open(os.path.expanduser(f"~/.cache/kdcache/words/{fname}")) as f:
        try:
            table = "en" if regex.match("^[A-Za-z0-9 -.?]+$", fname) else "ch"
            j = f.read()
            r = cursor.execute(
                f"INSERT OR REPLACE INTO {table} (query, detail, update_time) VALUES(?, ?, ?)",
                [
                    fname,
                    j,
                    datetime.datetime.now(),
                ]
            )
            print("inserted", fname)
        except Exception as e:
            print("Failed to parse json", fname, e)

db.commit()
