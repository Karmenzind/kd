#!/usr/bin/env python
# -*- coding: utf-8 -*-
import json

import sqlite3

db = sqlite3.connect("./kd_data.db")

cur = db.cursor()

qs = []

to_del = []

def shrink(d):
    to_pop = []
    for k in d:
        if not d[k]:
            to_pop.append(k)
    for k in to_pop:
        d.pop(k)


def fix_eg_key():
    params = []
    cur.execute("select query, detail from en")
    for q, s in cur:
        d = json.loads(s)
        if d.get("eg"):
            eg = {}
            for k, v in d["eg"].items():
                eg[k[:2]] = v

            # print(eg)
            # input()
            d['eg'] = eg

            j = json.dumps(d, separators=(',', ':'))
            params.append([j, q])
    if params:
        r = cur.executemany(
            "update en set detail = ? where query = ?",
            params,
        )
        print(r.fetchall())
        db.commit()



def fix_bi_eg():
    params = []
    cur.execute("select query, detail from en")
    for q, s in cur:
        d = json.loads(s)
        if d.get("eg") and d['eg'].get("bilingual"):
            for i in d['eg']['bilingual']:
                print("--------------------------------------------")
                print(i)
                i[0], i[1] = i[1], i[0]
                print(i)
            j = json.dumps(d, separators=(',', ':'))
            params.append([j, q])
    if params:
        r = cur.executemany(
            "update en set detail = ? where query = ?",
            params,
        )
        print(r.fetchall())
        db.commit()



def convert_keys():
    params = []
    cur.execute("select query, detail from en")
    for (q, detail) in cur:
        # print(q, detail)
        d = json.loads(detail)
        nd = {
            "k": d.get("Keyword"),
            "pron": d.get("Pronounce"),
            "para": d.get("Paraphrase"),
            "eg": d.get("Examples"),
        }
        shrink(nd)
        if d.get("Collins"):
            c = d["Collins"]
            nc = {
                "star": c.get("Star"),
                "rank": c.get("ViaRank"),
                "pat": c.get("AdditionalPattern"),
                "li": []
            }
            if nc["pat"]:
                nc["pat"] = nc["pat"].strip("()").replace(" ", "")
            for i in (c.get("Items") or []):
                ni = {
                    "a": i.get("Additional"),
                    "maj": i.get("MajorTrans"),
                    "eg": i.get("ExampleLists")
                }
                shrink(ni)
                if ni:
                    nc["li"].append(ni)

            shrink(nc)
            if nc:
                nd["co"] = nc

        # __import__('pprint').pprint(nd)
        j = json.dumps(nd, separators=(',', ':'))

        # r  = cur.execute("update en set detail = ? where query = ?", [j, q])
        params.append([j, q])


    cur.executemany(
        "update en set detail = ? where query = ?",
        params,
    )

    db.commit()

def main():
    """main function"""
    pass


if __name__ == "__main__":
    # fix_bi_eg()
    fix_eg_key()
    pass
