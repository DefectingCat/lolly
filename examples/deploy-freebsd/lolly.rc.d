#!/bin/sh
#
# lolly - High Performance HTTP Server and Reverse Proxy
#
# PROVIDE: lolly
# REQUIRE: LOGIN
# KEYWORD: shutdown

. /etc/rc.subr

name="lolly"
rcvar="lolly_enable"

load_rc_config $name

: ${lolly_enable:="NO"}
: ${lolly_config:="/usr/local/etc/lolly/lolly.yaml"}
: ${lolly_pidfile:="/var/run/lolly.pid"}

command="/usr/sbin/daemon"
pidfile="${lolly_pidfile}"
procname="/usr/local/sbin/lolly"

start_precmd="lolly_prestart"

lolly_prestart()
{
    [ -d /var/log/lolly ] || mkdir -p /var/log/lolly
    [ -d /var/db/lolly ] || mkdir -p /var/db/lolly
    [ -d /var/www/lolly ] || mkdir -p /var/www/lolly
    rm -f "${lolly_pidfile}"
}

command_args="-c -f -p ${lolly_pidfile} /usr/local/sbin/lolly -c ${lolly_config}"

extra_commands="reload rotate"
reload_cmd="lolly_reload"
rotate_cmd="lolly_rotate"

lolly_reload()
{
    if [ -f "${lolly_pidfile}" ]; then
        echo "Reloading lolly configuration..."
        kill -HUP $(cat "${lolly_pidfile}")
    else
        echo "lolly is not running"
        return 1
    fi
}

lolly_rotate()
{
    if [ -f "${lolly_pidfile}" ]; then
        echo "Rotating lolly logs..."
        kill -USR1 $(cat "${lolly_pidfile}")
    else
        echo "lolly is not running"
        return 1
    fi
}

run_rc_command "$1"
