# Security Policy

## Supported Versions

Please see [Releases](https://github.com/electroneum/electroneum-sc/releases). We recommend using the [most recently released version](https://github.com/electroneum/electroneum-sc/releases/latest).

## Audit reports

Audit reports are published in the `docs` folder: https://github.com/electroneum/electroneum-sc/tree/master/docs/audits 

| Scope | Date | Report Link |
| ------- | ------- | ----------- |
| `geth` | 20170425 | [pdf](https://github.com/ethereum/go-ethereum/blob/master/docs/audits/2017-04-25_Geth-audit_Truesec.pdf) |
| `clef` | 20180914 | [pdf](https://github.com/ethereum/go-ethereum/blob/master/docs/audits/2018-09-14_Clef-audit_NCC.pdf) |
| `Discv5` | 20191015 | [pdf](https://github.com/ethereum/go-ethereum/blob/master/docs/audits/2019-10-15_Discv5_audit_LeastAuthority.pdf) |
| `Discv5` | 20200124 | [pdf](https://github.com/ethereum/go-ethereum/blob/master/docs/audits/2020-01-24_DiscV5_audit_Cure53.pdf) |

## Reporting a Vulnerability

**Please do not file a public ticket** mentioning the vulnerability. Instead, contact one of the blockchain team directly.

Please read the [disclosure page](https://github.com/electroneum/electroneum-sc/security/advisories?state=published) for more information about publicly disclosed security vulnerabilities.

Use the built-in `etn-sc version-check` feature to check whether the software is affected by any known vulnerability. This command will fetch the latest [`vulnerabilities.json`](https://geth.ethereum.org/docs/vulnerabilities/vulnerabilities.json) file which contains known security vulnerabilities concerning `etn-sc`, and cross-check the data against its own version number.

The following key may be used to communicate sensitive information to developers.

Fingerprint: `0B8F 5DEE FABF 563F 4D45  A217 CC48 99F1 3749 1B2F`

```
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQGNBGLeZHMBDAC3ptnfWFm6Z7sTS//lpZMxxjxGhTZfeC4770PAjYkWdMSw8uAS
ANvu/BAEOb2g4/nmHQJYqRsuF2+TWapzTFfaaX3VRba7DMJ5Pf6xNYCkP7rIP0X6
BOiqgGFss62RBCKIHjXPFnaJWcOFDPkY8bZ3S9fSugJxRkigncA7XLoG8VF0w3rT
/Z4MqpBjIJLmEImsbfPw6c+eLs7oD2zBuGkI69mot6onDhMpOnE3QqFEPT9ta820
EpZnkksHEBF0YD24vFaGHgAEksYLMQA4igUCGB5UYQg+lebTGQ1eGPcW+qwVmSGw
vGrelBtyxty1j92atxRfCkUQ/YbqBIAB2CECSVok03hHWcZCkf69A4lqui74pgLf
iazfCvYRdcqbBUiuZBj/bNzLuCDnt4bP4l6EJI7LugMsAs+eruhdIJoC0+fgrTYG
0i5bMO4zTuvFWw7Zctz+u02lqfz+xvG8NdMaEIipJNkQwfys4emi+Sj08e8ueFq9
hOt/88F8UKUjlVEAEQEAAbQ9Q2hyaXN0b3BoZXIgQ2hhcmxlcyBIYXJyaXNvbiA8
Y2hyaXMuaGFycmlzb25AZWxlY3Ryb25ldW0uY29tPokBzgQTAQoAOBYhBAuPXe76
v1Y/TUWiF8xImfE3SRsvBQJi3mRzAhsDBQsJCAcCBhUKCQgLAgQWAgMBAh4BAheA
AAoJEMxImfE3SRsvHPoL/3nehlcd7imL+yvVNTuOMciWC+4pIp3EIBhNql6Wy16/
URDTVeyB5ixb1A8Z3Ohdtld0jhE6yGWQ9dR62V2QHBX5D9z3bL2k0fz+49l2v0HA
u2YPRK1LLpaDUN72Lxj++owdBDadpyEFvJ/gq6gs8pLTvuXp/rNEtUN/7VhgCTW+
o1v0ulDmkWGhFcSdbP9RPbNFKHetjbQGu0uQ6FkljKk/O6ZrPIbdPijgSXab2H3c
eJ0qLMFznS3v8bzN/klaZfPAVhVGFt2usdZSdU82UCP8kCNPLcJ3ISWgmwOrAUgh
UW0HgBTRjyODPbDGFparHRwPJbqlkSq1LSMePGQYWyqNzL2HyGv5LYeviZ3sTuYs
LS2GVPmG1VXqTiwILpOBPoIMSWPR0/ugKYNSfE9h6GEeQZsBHbNFlsfrisJztl8X
S2fCMJ9JmPrrjuC0v0UzkfCHqJDHYZzh3+6kHsH6HdrDNybWjn1+FaOVB7ENR+lI
uw/M+NeS/7RmSfprFBtfvrkBjQRi3mRzAQwAs/40LMUxRl9xMcyBYRyzaiUNVJRH
cBcf2VBvr3FnMWS8MTXO+VP0gpqxvtj5nyVwovH+k8XTb9roCCOmcnzbSP+PhXyv
cU4bAi+Q+2hcVWD7lLgWpuCNAbSEBZzmf+k38QrVe+jWq+KSrrbyjIHOdGQq2qGo
3zZ3cHRL/Np1OrJ7/wYtMtZTPAN0/DnBEy88k2zvzMb4VgZdWwRCPMciNqqNGs09
VixVBjBtsjcn6Bd7NBZGyWQRKcl+SJrcs2rA/xkYdlCMfYCH8zngWSIBfc5mslcR
fVCXOAJfGhiP/PAdGXn40zloMH/5hy09XnSi3fMKrAn6A/JnwE8hd3sSJl7aL0mH
guudgGEzFsIaNH1jR9m8T2h2752acqb1rp41GR+7xEiWJo8sQcBrJ97/UZOvDcZT
u39QMvueJyWvghRNJUMf5TUMAdTPDhnTLr8iA7iHstjBzNPnmGey2977zZGEmHn/
tPT0lsqlw8XNmcsCY49mNOkQZirsNc8eH8OBABEBAAGJAbYEGAEKACAWIQQLj13u
+r9WP01FohfMSJnxN0kbLwUCYt5kcwIbDAAKCRDMSJnxN0kbLyLXC/4sHLBubWll
zvSFhyqCqozoppuQ2L+vyiLYjPd33A1JewkVDo2UPjh8RIQVBpBmErXeJf8B8CdC
S4UGqf0MBjUyWQ3m3kk26oMnxDuqfFRKb2uliEAMeOUP2WPvOcZgnv+KMq26yxPc
qQ1z1xm/OZmcZsp6pGp9p6aAmsD+CxL1wksgbWwAo1NXA8k9Q33AK7wHu6eWhjF6
5bKvIDFUUPuJcf37KkP9Pza9THsIJPF49Ub7zSIOFxf565LEDL8EmMBwo7vT/DcX
3U4et+czRkroRP5xRtNMGWo/WX1ig9i8GwJcA3p/5A8k48pKRJHs36NPa91OGvGE
syyrQve++pJspQ8s4vUdv0FyJrCGI6Qg5Zzequ6SpBVBa0kdv/wjxcPOlFpnSitw
h3Hx8StF2+sYYp9b2SrS6aa2/TLXxClRTgYNt15XuaLyT23zF2KpnFFKW2zx06nE
Ijn0ERV9kkA4MvB/37KXIKpEgWgPm7f2NEXR9zNmpuu7b916KmO99q4=
=CE/G
-----END PGP PUBLIC KEY BLOCK-----
```

If you find a vulnerability in any of the sister projects of the Electroneum Smart Chain, such as

- the main Electroneum website (https://electroneum.com/) and it's API (https://api.electroneum.com/);
- the MyElectroneum wallet system (https://my.electroneum.com/);
- the freelancing platform, Anytask (https://www.anytask.com/) and it's API (https://api.anytask.com/);
- the mobile apps, IOS (https://apps.apple.com/us/app/electroneum/id1270774992) and Android (https://play.google.com/store/apps/details?id=com.electroneum.mobile&hl=en_US)

please report these via **BugCrowd ONLY** (https://bugcrowd.com/electroneum).