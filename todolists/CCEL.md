```bash
./deploy-foo.sh

=== [1/6] 构建 ===
Hash algorithm:  	sha256
Salt:            	0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
Root hash:      	61633bf7df91d6add2a8c93b8669b2b47601b12ab0e8c3e2b88d03749af9b644
Hash device size: 	1425408 [bytes]
‣  /root/zeroseal-mkosi/zeroseal size is 188.0M.
=== root hash: 61633bf7df91d6add2a8c93b8669b2b47601b12ab0e8c3e2b88d03749af9b644 ===
=== [2/6] dd 推盘 ===
42+1 records in
42+1 records out
179380224 bytes (179 MB, 171 MiB) copied, 0.568304 s, 316 MB/s
0+1 records in
0+1 records out
1425408 bytes (1.4 MB, 1.4 MiB) copied, 0.00324414 s, 439 MB/s
=== [3/6] verity 验证 ===
verity open 退出码: 0
=== [4/6] 打 UKI ===
Kernel version not specified, starting autodetection 😖.
Found uname version: 6.8.0-111-generic
Wrote unsigned zeroseal.efi
UKI: fdd1cc08c14dfc1ba6c9f027d0cb1284ba499446851d2005f1b5557fa8f39c47  zeroseal.efi
=== [5/6] 写 grub menuentry ===
done
=== [6/6] grub-reboot ===
next_entry: next_entry=ZeroSeal UKI
=== 部署完成。去控制台关机+开机。===
(mkosi-venv) root@iZ2ze1lahuiqzz66gx26ouZ:~/zeroseal-mkosi# ls -l
total 354368
-rwxr-xr-x  1 root root      2246 Jun 18 18:29 deploy-foo.sh
-rwxr-xr-x  1 root root      1852 Jun 15 20:51 deploy.sh
-rw-r--r--  1 root root       102 Jun 18 18:14 foo.py
lrwxrwxrwx  1 root root        15 Jun 18 18:33 initrd -> initrd.cpio.zst
-rw-r--r--  1 root root  31582174 Jun 18 18:33 initrd.cpio.zst
-rw-r--r--  1 root root       482 Jun 15 16:57 mkosi.conf
-rwxr-xr-x  1 root root       587 Jun  6 18:56 mkosi.postinst.chroot
-rwxr-xr-x  1 root root       617 Jun  6 18:56 mkosi.postoutput
drwxr-xr-x 21 root root      4096 Jan  1  1970 mkosi.tools
-rw-r--r--  1 root root      2366 Jun 15 16:11 mkosi.tools.manifest
drwxr-xr-x  5 root root      4096 Jun  6 18:43 overlay
-rwxr-xr-x  1 root root       179 Jun 15 16:05 readdiag.sh
drwxr-xr-x 21 root root      4096 Jan  1  1970 zeroseal
-rw-r--r--  1 root root  75248640 Jun 18 18:33 zeroseal.efi
-rw-r--r--  1 root root 179380224 Jun 18 18:33 zeroseal.erofs
-rw-r--r--  1 root root  60130940 Jun 18 18:33 zeroseal.initrd
-rw-------  1 root root        64 Jun 18 18:33 zeroseal.roothash
-rw-------  1 root root   1425408 Jun 18 18:33 zeroseal.verity
-rw-------  1 root root  15047048 Jun 18 18:33 zeroseal.vmlinuz
(mkosi-venv) root@iZ2ze1lahuiqzz66gx26ouZ:~/zeroseal-mkosi#
```





```bash
python3 ccel_dump.py ccel-test0.bin
==== TDX Event Log Entry - 0 [0xBAE1E000] ====
RTMR              : 0
Type              : 3 (EV_NO_ACTION)
Length            : 65
Algorithms Number : 1
  Algorithms[0xC] Size: 384
RAW DATA: ----------------------------------------------
BAE1E000  01 00 00 00 03 00 00 00 00 00 00 00 00 00 00 00  ................
BAE1E010  00 00 00 00 00 00 00 00 00 00 00 00 21 00 00 00  ............!...
BAE1E020  53 70 65 63 20 49 44 20 45 76 65 6E 74 30 33 00  Spec ID Event03.
BAE1E030  00 00 00 00 00 02 00 02 01 00 00 00 0C 00 30 00  ..............0.
BAE1E040  00                                               .
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 1 [0xBAE1E041] ====
RTMR              : 0
Type              : 0x8000000B (EV_EFI_HANDOFF_TABLES2)
Length            : 108
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 277e6934803a7b4b21cae32b6eebe2b154462852510ad90cf0242f95627c5e0add22b63fbbc7dbca10ad18241b1adb54
Event Data        : TdxTable
RAW DATA: ----------------------------------------------
BAE1E041  01 00 00 00 0B 00 00 80 01 00 00 00 0C 00 27 7E  ..............'~
BAE1E051  69 34 80 3A 7B 4B 21 CA E3 2B 6E EB E2 B1 54 46  i4.:{K!..+n...TF
BAE1E061  28 52 51 0A D9 0C F0 24 2F 95 62 7C 5E 0A DD 22  (RQ....$/.b|^.."
BAE1E071  B6 3F BB C7 DB CA 10 AD 18 24 1B 1A DB 54 2A 00  .?.......$...T*.
BAE1E081  00 00 09 54 64 78 54 61 62 6C 65 00 01 00 00 00  ...TdxTable.....
BAE1E091  00 00 00 00 AF 96 BB 93 F2 B9 B8 4E 94 62 E0 BA  ...........N.b..
BAE1E0A1  74 56 42 36 00 90 80 00 00 00 00 00              tVB6........
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 2 [0xBAE1E0AD] ====
RTMR              : 0
Type              : 0x8000000A (EV_EFI_PLATFORM_FIRMWARE_BLOB2)
Length            : 124
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 5a9a6faadec79debc1d35d4ce1d2d55f1089f1db50b0ad5eaa449112f3cdf4a5d08b2f4ebb38365fde7a347470e63a89
Event Data        : )Fv(XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX)
RAW DATA: ----------------------------------------------
BAE1E0AD  01 00 00 00 0A 00 00 80 01 00 00 00 0C 00 5A 9A  ..............Z.
BAE1E0BD  6F AA DE C7 9D EB C1 D3 5D 4C E1 D2 D5 5F 10 89  o.......]L..._..
BAE1E0CD  F1 DB 50 B0 AD 5E AA 44 91 12 F3 CD F4 A5 D0 8B  ..P..^.D........
BAE1E0DD  2F 4E BB 38 36 5F DE 7A 34 74 70 E6 3A 89 3A 00  /N.86_.z4tp.:.:.
BAE1E0ED  00 00 29 46 76 28 58 58 58 58 58 58 58 58 2D 58  ..)Fv(XXXXXXXX-X
BAE1E0FD  58 58 58 2D 58 58 58 58 2D 58 58 58 58 2D 58 58  XXX-XXXX-XXXX-XX
BAE1E10D  58 58 58 58 58 58 58 58 58 58 29 00 00 00 C0 FF  XXXXXXXXXX).....
BAE1E11D  00 00 00 00 00 00 04 00 00 00 00 00              ............
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 3 [0xBAE1E129] ====
RTMR              : 0
Type              : 0x1 (EV_POST_CODE)
Length            : 75
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : a517a3e386aeb8cfa67d0ff9d0d7e90119f07ac6003816826e785fa21ba6015dded835ab96ffcd77e2d48271f3920533
Event Data        : ACPI DATA
RAW DATA: ----------------------------------------------
BAE1E129  01 00 00 00 01 00 00 00 01 00 00 00 0C 00 A5 17  ................
BAE1E139  A3 E3 86 AE B8 CF A6 7D 0F F9 D0 D7 E9 01 19 F0  .......}........
BAE1E149  7A C6 00 38 16 82 6E 78 5F A2 1B A6 01 5D DE D8  z..8..nx_....]..
BAE1E159  35 AB 96 FF CD 77 E2 D4 82 71 F3 92 05 33 09 00  5....w...q...3..
BAE1E169  00 00 41 43 50 49 20 44 41 54 41                 ..ACPI DATA
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 4 [0xBAE1E174] ====
RTMR              : 0
Type              : 0xA (EV_PLATFORM_CONFIG_FLAGS)
Length            : 83
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : a9c92209e13127b1333b1762820ad9c5eca921e42bcf84d680b42b7616401419eb7ae7b552fad8a050f88bad6584a58e
Event Data        : etc/table-loader
RAW DATA: ----------------------------------------------
BAE1E174  01 00 00 00 0A 00 00 00 01 00 00 00 0C 00 A9 C9  ................
BAE1E184  22 09 E1 31 27 B1 33 3B 17 62 82 0A D9 C5 EC A9  "..1'.3;.b......
BAE1E194  21 E4 2B CF 84 D6 80 B4 2B 76 16 40 14 19 EB 7A  !.+.....+v.@...z
BAE1E1A4  E7 B5 52 FA D8 A0 50 F8 8B AD 65 84 A5 8E 11 00  ..R...P...e.....
BAE1E1B4  00 00 65 74 63 2F 74 61 62 6C 65 2D 6C 6F 61 64  ..etc/table-load
BAE1E1C4  65 72 00                                         er.
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 5 [0xBAE1E1C7] ====
RTMR              : 0
Type              : 0xA (EV_PLATFORM_CONFIG_FLAGS)
Length            : 122
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 527d5706660d797e296ab55b9d51358c2ebfcef78470aa1924caad21382b63b5b4bc14ab7495e518daf22f1bdabba638
Event Data        : etc/acpi/rsdp
RAW DATA: ----------------------------------------------
BAE1E1C7  01 00 00 00 0A 00 00 00 01 00 00 00 0C 00 52 7D  ..............R}
BAE1E1D7  57 06 66 0D 79 7E 29 6A B5 5B 9D 51 35 8C 2E BF  W.f.y~)j.[.Q5...
BAE1E1E7  CE F7 84 70 AA 19 24 CA AD 21 38 2B 63 B5 B4 BC  ...p..$..!8+c...
BAE1E1F7  14 AB 74 95 E5 18 DA F2 2F 1B DA BB A6 38 38 00  ..t...../....88.
BAE1E207  00 00 65 74 63 2F 61 63 70 69 2F 72 73 64 70 00  ..etc/acpi/rsdp.
BAE1E217  00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
BAE1E227  00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
BAE1E237  00 00 00 00 00 00 00 00 00 00                    ..........
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 6 [0xBAE1E241] ====
RTMR              : 0
Type              : 0xA (EV_PLATFORM_CONFIG_FLAGS)
Length            : 122
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 7f0e9f683c97bb06295f880610b6d94896bf445df05784c164d5abfcb3ffa8accdb3e8a8e97aa1f670bc190ddfb2e9ab
Event Data        : etc/acpi/tables
RAW DATA: ----------------------------------------------
BAE1E241  01 00 00 00 0A 00 00 00 01 00 00 00 0C 00 7F 0E  ................
BAE1E251  9F 68 3C 97 BB 06 29 5F 88 06 10 B6 D9 48 96 BF  .h<...)_.....H..
BAE1E261  44 5D F0 57 84 C1 64 D5 AB FC B3 FF A8 AC CD B3  D].W..d.........
BAE1E271  E8 A8 E9 7A A1 F6 70 BC 19 0D DF B2 E9 AB 38 00  ...z..p.......8.
BAE1E281  00 00 65 74 63 2F 61 63 70 69 2F 74 61 62 6C 65  ..etc/acpi/table
BAE1E291  73 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  s...............
BAE1E2A1  00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
BAE1E2B1  00 00 00 00 00 00 00 00 00 00                    ..........
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 7 [0xBAE1E2BB] ====
RTMR              : 1
Type              : 0x80000003 (EV_EFI_BOOT_SERVICES_APPLICATION)
Length            : 230
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 4637fb5cd30847e5f09ae24f8a50ce1611c4d21afd0ecb69c8ec40bc82dc11bc48abda1f8044fe340bfb70b29606eb47
Event Data        : npK
RAW DATA: ----------------------------------------------
BAE1E2BB  02 00 00 00 03 00 00 80 01 00 00 00 0C 00 46 37  ..............F7
BAE1E2CB  FB 5C D3 08 47 E5 F0 9A E2 4F 8A 50 CE 16 11 C4  .\..G....O.P....
BAE1E2DB  D2 1A FD 0E CB 69 C8 EC 40 BC 82 DC 11 BC 48 AB  .....i..@.....H.
BAE1E2EB  DA 1F 80 44 FE 34 0B FB 70 B2 96 06 EB 47 A4 00  ...D.4..p....G..
BAE1E2FB  00 00 18 F0 F1 B9 00 00 00 00 08 C0 0E 00 00 00  ................
BAE1E30B  00 00 00 00 00 00 00 00 00 00 84 00 00 00 00 00  ................
BAE1E31B  00 00 02 01 0C 00 D0 41 03 0A 00 00 00 00 01 01  .......A........
BAE1E32B  06 00 00 03 03 17 10 00 01 00 00 00 00 00 00 00  ................
BAE1E33B  00 00 00 00 04 01 2A 00 02 00 00 00 00 10 00 00  ......*.........
BAE1E34B  00 00 00 00 00 40 06 00 00 00 00 00 F2 F8 1F BF  .....@..........
BAE1E35B  8D BB 61 47 A7 0E 08 E6 6E 70 4B F3 02 02 04 04  ..aG....npK.....
BAE1E36B  34 00 5C 00 45 00 46 00 49 00 5C 00 75 00 62 00  4.\.E.F.I.\.u.b.
BAE1E37B  75 00 6E 00 74 00 75 00 5C 00 73 00 68 00 69 00  u.n.t.u.\.s.h.i.
BAE1E38B  6D 00 78 00 36 00 34 00 2E 00 65 00 66 00 69 00  m.x.6.4...e.f.i.
BAE1E39B  00 00 7F FF 04 00                                ......
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 8 [0xBAE1E3A1] ====
RTMR              : 2
Type              : 0xD (EV_IPL)
Length            : 74
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 053357ea65185f010b8caa1fc265cfd5e80c7cc781254fa3f1e5ea9d345a87003cf761472a2f0423f15297f55cfe248f
Event Data        : MokList
RAW DATA: ----------------------------------------------
BAE1E3A1  03 00 00 00 0D 00 00 00 01 00 00 00 0C 00 05 33  ...............3
BAE1E3B1  57 EA 65 18 5F 01 0B 8C AA 1F C2 65 CF D5 E8 0C  W.e._......e....
BAE1E3C1  7C C7 81 25 4F A3 F1 E5 EA 9D 34 5A 87 00 3C F7  |..%O.....4Z..<.
BAE1E3D1  61 47 2A 2F 04 23 F1 52 97 F5 5C FE 24 8F 08 00  aG*/.#.R..\.$...
BAE1E3E1  00 00 4D 6F 6B 4C 69 73 74 00                    ..MokList.
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 9 [0xBAE1E3EB] ====
RTMR              : 2
Type              : 0xD (EV_IPL)
Length            : 75
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 80ee2571334a57bf90238d21964447e542079d4805fa87887817a97dcb720906683a09b1ac634c76c0c0be1177f76110
Event Data        : MokListX
RAW DATA: ----------------------------------------------
BAE1E3EB  03 00 00 00 0D 00 00 00 01 00 00 00 0C 00 80 EE  ................
BAE1E3FB  25 71 33 4A 57 BF 90 23 8D 21 96 44 47 E5 42 07  %q3JW..#.!.DG.B.
BAE1E40B  9D 48 05 FA 87 88 78 17 A9 7D CB 72 09 06 68 3A  .H....x..}.r..h:
BAE1E41B  09 B1 AC 63 4C 76 C0 C0 BE 11 77 F7 61 10 09 00  ...cLv....w.a...
BAE1E42B  00 00 4D 6F 6B 4C 69 73 74 58 00                 ..MokListX.
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 10 [0xBAE1E436] ====
RTMR              : 0
Type              : 0x800000E0 (EV_EFI_VARIABLE_AUTHORITY)
Length            : 134
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : f143e2948d63fcd3442e841bb36a7e180871f0a8946541961fe9d12e70d0727874600956264dba531e2edd8729c5eb38
Event Data        : sbat,1,2021030218
RAW DATA: ----------------------------------------------
BAE1E436  01 00 00 00 E0 00 00 80 01 00 00 00 0C 00 F1 43  ...............C
BAE1E446  E2 94 8D 63 FC D3 44 2E 84 1B B3 6A 7E 18 08 71  ...c..D....j~..q
BAE1E456  F0 A8 94 65 41 96 1F E9 D1 2E 70 D0 72 78 74 60  ...eA.....p.rxt`
BAE1E466  09 56 26 4D BA 53 1E 2E DD 87 29 C5 EB 38 44 00  .V&M.S....)..8D.
BAE1E476  00 00 50 AB 5D 60 46 E0 00 43 AB B6 3D D8 10 DD  ..P.]`F..C..=...
BAE1E486  8B 23 09 00 00 00 00 00 00 00 12 00 00 00 00 00  .#..............
BAE1E496  00 00 53 00 62 00 61 00 74 00 4C 00 65 00 76 00  ..S.b.a.t.L.e.v.
BAE1E4A6  65 00 6C 00 73 62 61 74 2C 31 2C 32 30 32 31 30  e.l.sbat,1,20210
BAE1E4B6  33 30 32 31 38 0A                                30218.
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 11 [0xBAE1E4BC] ====
RTMR              : 2
Type              : 0xD (EV_IPL)
Length            : 81
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 8d2ce87d86f55fcfab770a047b090da23270fa206832dfea7e0c946fff451f819add242374be551b0d6318ed6c7d41d8
Event Data        : MokListTrusted
RAW DATA: ----------------------------------------------
BAE1E4BC  03 00 00 00 0D 00 00 00 01 00 00 00 0C 00 8D 2C  ...............,
BAE1E4CC  E8 7D 86 F5 5F CF AB 77 0A 04 7B 09 0D A2 32 70  .}.._..w..{...2p
BAE1E4DC  FA 20 68 32 DF EA 7E 0C 94 6F FF 45 1F 81 9A DD  . h2..~..o.E....
BAE1E4EC  24 23 74 BE 55 1B 0D 63 18 ED 6C 7D 41 D8 0F 00  $#t.U..c..l}A...
BAE1E4FC  00 00 4D 6F 6B 4C 69 73 74 54 72 75 73 74 65 64  ..MokListTrusted
BAE1E50C  00                                               .
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 12 [0xBAE1E50D] ====
RTMR              : 1
Type              : 0x80000003 (EV_EFI_BOOT_SERVICES_APPLICATION)
Length            : 154
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : d9c40784e214bb829477f46245758e74f6b145dbf012960d4053c2fe27545738d89833297b4fd9ec348dde75910bfa33
Event Data        : (
RAW DATA: ----------------------------------------------
BAE1E50D  02 00 00 00 03 00 00 80 01 00 00 00 0C 00 D9 C4  ................
BAE1E51D  07 84 E2 14 BB 82 94 77 F4 62 45 75 8E 74 F6 B1  .......w.bEu.t..
BAE1E52D  45 DB F0 12 96 0D 40 53 C2 FE 27 54 57 38 D8 98  E.....@S..'TW8..
BAE1E53D  33 29 7B 4F D9 EC 34 8D DE 75 91 0B FA 33 58 00  3){O..4..u...3X.
BAE1E54D  00 00 18 E0 BB B9 00 00 00 00 88 97 28 00 00 00  ............(...
BAE1E55D  00 00 00 00 00 00 00 00 00 00 38 00 00 00 00 00  ..........8.....
BAE1E56D  00 00 04 04 34 00 5C 00 45 00 46 00 49 00 5C 00  ....4.\.E.F.I.\.
BAE1E57D  75 00 62 00 75 00 6E 00 74 00 75 00 5C 00 67 00  u.b.u.n.t.u.\.g.
BAE1E58D  72 00 75 00 62 00 78 00 36 00 34 00 2E 00 65 00  r.u.b.x.6.4...e.
BAE1E59D  66 00 69 00 00 00 7F FF 04 00                    f.i.......
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 13 [0xBAE1E5A7] ====
RTMR              : 1
Type              : 0x80000003 (EV_EFI_BOOT_SERVICES_APPLICATION)
Length            : 122
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : baaf57ae19efbb1321e7fc28c6ee8752d3fd4cf0c1c1186d5a093f0b4c4ed155b773203fa41a80ab8dfab529a5f89469
Event Data        : 0R
RAW DATA: ----------------------------------------------
BAE1E5A7  02 00 00 00 03 00 00 80 01 00 00 00 0C 00 BA AF  ................
BAE1E5B7  57 AE 19 EF BB 13 21 E7 FC 28 C6 EE 87 52 D3 FD  W.....!..(...R..
BAE1E5C7  4C F0 C1 C1 18 6D 5A 09 3F 0B 4C 4E D1 55 B7 73  L....mZ.?.LN.U.s
BAE1E5D7  20 3F A4 1A 80 AB 8D FA B5 29 A5 F8 94 69 38 00   ?.......)...i8.
BAE1E5E7  00 00 00 60 D1 B1 00 00 00 00 88 99 E5 00 00 00  ...`............
BAE1E5F7  00 00 00 00 00 00 00 00 00 00 18 00 00 00 00 00  ................
BAE1E607  00 00 04 03 14 00 F8 D1 C5 55 CD 04 B5 46 8A 20  .........U...F.
BAE1E617  E5 6C BB 30 52 D0 7F FF 04 00                    .l.0R.....
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 14 [0xBAE1E621] ====
RTMR              : 2
Type              : 0x6 (EV_EVENT_TAG)
Length            : 100
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 914d914a645fc37c4d2d50ddda67c039b5b78eba6946bb838b0b04fb1908dc378cb166b59aeb5da70dc914a7ae5c311e
Event Data        : LOADED_IMAGE::LoadOptions
RAW DATA: ----------------------------------------------
BAE1E621  03 00 00 00 06 00 00 00 01 00 00 00 0C 00 91 4D  ...............M
BAE1E631  91 4A 64 5F C3 7C 4D 2D 50 DD DA 67 C0 39 B5 B7  .Jd_.|M-P..g.9..
BAE1E641  8E BA 69 46 BB 83 8B 0B 04 FB 19 08 DC 37 8C B1  ..iF.........7..
BAE1E651  66 B5 9A EB 5D A7 0D C9 14 A7 AE 5C 31 1E 22 00  f...]......\1.".
BAE1E661  00 00 ED 22 3B 8F 1A 00 00 00 4C 4F 41 44 45 44  ...";.....LOADED
BAE1E671  5F 49 4D 41 47 45 3A 3A 4C 6F 61 64 4F 70 74 69  _IMAGE::LoadOpti
BAE1E681  6F 6E 73 00                                      ons.
RAW DATA: ----------------------------------------------

==== TDX Event Log Entry - 15 [0xBAE1E685] ====
RTMR              : 2
Type              : 0x6 (EV_EVENT_TAG)
Length            : 87
Algorithms ID     : 12 (TPM_ALG_SHA384)
Digest[0] : 66b4763797e510fb9fcc0fc104b31644bde8b5ec33e347a61ba693146fd03e5277314beee2a3ce1e94971af0bb480305
Event Data        : Linux initrd
RAW DATA: ----------------------------------------------
BAE1E685  03 00 00 00 06 00 00 00 01 00 00 00 0C 00 66 B4  ..............f.
BAE1E695  76 37 97 E5 10 FB 9F CC 0F C1 04 B3 16 44 BD E8  v7...........D..
BAE1E6A5  B5 EC 33 E3 47 A6 1B A6 93 14 6F D0 3E 52 77 31  ..3.G.....o.>Rw1
BAE1E6B5  4B EE E2 A3 CE 1E 94 97 1A F0 BB 48 03 05 15 00  K..........H....
BAE1E6C5  00 00 EC 22 3B 8F 0D 00 00 00 4C 69 6E 75 78 20  ...";.....Linux
BAE1E6D5  69 6E 69 74 72 64 00                             initrd.
RAW DATA: ----------------------------------------------

root@iZ2ze1lahuiqzz66gx26ouZ:~# cd zeroseal-mkosi/
root@iZ2ze1lahuiqzz66gx26ouZ:~/zeroseal-mkosi# python3 foo.py
Traceback (most recent call last):
  File "/root/zeroseal-mkosi/foo.py", line 1, in <module>
    import lief
ModuleNotFoundError: No module named 'lief'
root@iZ2ze1lahuiqzz66gx26ouZ:~/zeroseal-mkosi# . ~/mkosi-venv/bin/activate
(mkosi-venv) root@iZ2ze1lahuiqzz66gx26ouZ:~/zeroseal-mkosi# python3 foo.py
4c54960bc272d6a9a7f7d5c101124e6e5d37a9e68a56f356b38891b212c3961da988e14054b7482ea85a40723ecb6284
(mkosi-venv) root@iZ2ze1lahuiqzz66gx26ouZ:~/zeroseal-mkosi#

```

