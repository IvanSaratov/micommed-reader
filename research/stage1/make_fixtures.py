"""Generate artificial 2001 records from constants. Reads no archive or real data."""
from datetime import datetime, timedelta
from pathlib import Path
import struct

FIX = Path(__file__).resolve().parents[1] / 'fixtures'


def record(origin, pressure, samples, events, minutes):
    b = bytearray(9312)

    def put(offset, fmt, values):
        struct.pack_into('<'+str(len(values))+fmt,b,offset,*values)

    def moment(offset, value):
        struct.pack_into('<H5B', b, offset, value.year,value.month,value.day,value.hour,value.minute,value.second)

    pt, pa, pb = zip(*pressure)
    put(0, 'H', [len(pressure)])
    put(8,'H',pa)
    put(0x328,'H',pb)
    put(0x648,'H',pt)
    put(0x968,'H',[len(events)])
    put(0x96a,'B',[code for _,code in events])
    put(0xa00,'H',[t for t,_ in events])
    first=origin+timedelta(minutes=pt[0])
    moment(0xb30,first)
    moment(0xb38,origin+timedelta(minutes=pt[-1]))
    for code, off in [(101,0xe08),(102,0xe04),(103,0xe06),(104,0xe0a)]:
        put(off,'H',[sum(c==code for _,c in events)])
    oa=sum(c==102 for _,c in events)*60/minutes
    put(0xdc8,'f',[oa,oa,0,0,oa,minutes,minutes])
    columns=list(zip(*samples))
    for i,(count,base,times,fmt) in enumerate([(0xf18,0xf20,0x104c,'B'),(0x12f4,0x12f8,0x153c,'H'),
            (0x1780,0x1782,0x18a4,'B'),(0x1ae8,0x1aea,0x1c0c,'B'),(0x1e50,0x1e52,0x1f74,'B')]):
        put(count,'H',[len(samples)])
        put(base,fmt,columns[i+1])
        put(times,'H',columns[0])
    return b


def main():
    origin=datetime(2001,2,3,9)
    zero=[(0,0,0,0,0,0)]
    smallest=record(origin,[(0,0,7)],zero,[],1)
    # Nonzero unused slots prove a decoder stops at the count, including zero events.
    struct.pack_into('<H',smallest,10,65000)
    smallest[0x96a]=102
    smallest[0x1e53]=255
    clock=record(origin,[(895,71,101),(899,72,102),(900,0,103),(900,76,104),(1439,95,105)],
                 [(895,1,501,7,13,21),(899,2,502,8,14,0),(900,3,503,9,15,23),(1439,4,504,10,16,24)],
                 [(900,102),(900,238),(1439,101)],30)
    full=record(origin,[(i,30+i,50+i) for i in range(400)],
                [(399*i//289,i%100,1000+i,5,12,i%100) for i in range(290)],
                [(i,101) for i in range(150)],390)
    blobs={'minimum':smallest,'clock':clock,'next-day':record(origin+timedelta(days=1),[(0,0,7)],zero,[],1),
           'at-capacity':full,'truncated':clock[:-1],'foreign-zero':bytes(9312)}
    for name, offset, fmt, value in [
            ('bad-count',0,'H',401),('bad-event-count',0x968,'H',151),('bad-channel-count',0x1e50,'H',291),
            ('bad-clock',0x648,'H',1440),('bad-date',0xb32,'B',13),('counter-mismatch',0xe04,'H',2),
            ('nan',0xdc8,'f',float('nan')),('unobserved-empty',0,'H',0)]:
        mutated=bytearray(clock)
        struct.pack_into('<'+fmt,mutated,offset,value)
        blobs[name]=mutated
    for name,blob in blobs.items():
        (FIX/(name+'.bin')).write_bytes(blob)
    print(f'{len(blobs)} synthetic files generated')


if __name__=='__main__':
    main()
